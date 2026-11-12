package monitoringprofile

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/discovery"
	"gopkg.in/yaml.v3"
)

const (
	profileSchemaVersion = "ar-imms/v1alpha1"
	maxProfileFileBytes  = 1 << 20
	maxProfileFiles      = 128
	maxProfileTotalBytes = 4 << 20
)

var environmentNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

type profileDocument struct {
	APIVersion string          `yaml:"api_version"`
	Profiles   []profileRecord `yaml:"profiles"`
}

type profileRecord struct {
	ID                   string            `yaml:"id"`
	Provider             string            `yaml:"provider"`
	ReceiverType         string            `yaml:"receiver_type"`
	RequiredCapabilities []string          `yaml:"required_capabilities"`
	MatchLabels          map[string]string `yaml:"match_labels"`
	CollectionInterval   string            `yaml:"collection_interval"`
	TargetPort           uint16            `yaml:"target_port"`
	TargetPortLabel      string            `yaml:"target_port_label"`
	MetricsPath          string            `yaml:"metrics_path"`
	UsernameEnv          string            `yaml:"username_env"`
	PasswordEnv          string            `yaml:"password_env"`
	TLSCAFile            string            `yaml:"tls_ca_file"`
}

// LoadDirectory loads bounded, versioned Agent monitoring profiles from the
// local config root. Only reviewed typed fields are accepted; files cannot
// provide Collector YAML, expressions, or executable extensions.
func LoadDirectory(configRoot string) (Registry, error) {
	profiles := []Profile{RedisProfile()}
	directory := filepath.Join(configRoot, "monitoring-profiles")
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return NewRegistry(profiles...)
	}
	if err != nil {
		return Registry{}, fmt.Errorf("read monitoring profile directory: %w", err)
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		extension := strings.ToLower(filepath.Ext(entry.Name()))
		if extension == ".yaml" || extension == ".yml" {
			paths = append(paths, filepath.Join(directory, entry.Name()))
		}
	}
	sort.Strings(paths)
	if len(paths) > maxProfileFiles {
		return Registry{}, fmt.Errorf("monitoring profile file count exceeds %d", maxProfileFiles)
	}
	totalBytes := int64(0)
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return Registry{}, fmt.Errorf("inspect monitoring profile file %q: %w", path, err)
		}
		totalBytes += info.Size()
		if totalBytes > maxProfileTotalBytes {
			return Registry{}, fmt.Errorf("monitoring profile directory exceeds %d bytes", maxProfileTotalBytes)
		}
		loaded, err := loadProfileFile(path)
		if err != nil {
			return Registry{}, err
		}
		profiles = append(profiles, loaded...)
		if len(profiles) > maxProfileFiles {
			return Registry{}, fmt.Errorf("monitoring profile count exceeds %d", maxProfileFiles)
		}
	}
	return NewRegistry(profiles...)
}

func loadProfileFile(path string) ([]Profile, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open monitoring profile file %q: %w", path, err)
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxProfileFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read monitoring profile file %q: %w", path, err)
	}
	if len(content) > maxProfileFileBytes {
		return nil, fmt.Errorf("monitoring profile file %q exceeds %d bytes", path, maxProfileFileBytes)
	}
	decoder := yaml.NewDecoder(strings.NewReader(string(content)))
	decoder.KnownFields(true)
	var document profileDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode monitoring profile file %q: %w", path, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("monitoring profile file %q must contain one YAML document", path)
	}
	if document.APIVersion != profileSchemaVersion || len(document.Profiles) == 0 {
		return nil, fmt.Errorf("monitoring profile file %q requires api_version %q and at least one profile", path, profileSchemaVersion)
	}
	profiles := make([]Profile, 0, len(document.Profiles))
	for _, record := range document.Profiles {
		profile, err := record.toProfile()
		if err != nil {
			return nil, fmt.Errorf("monitoring profile file %q: %w", path, err)
		}
		profiles = append(profiles, profile)
	}
	return profiles, nil
}

func (record profileRecord) toProfile() (Profile, error) {
	if record.Provider != "docker" {
		return Profile{}, fmt.Errorf("profile %q uses unsupported provider %q", record.ID, record.Provider)
	}
	if record.TargetPort == 0 {
		return Profile{}, fmt.Errorf("profile %q requires target_port", record.ID)
	}
	interval, err := time.ParseDuration(record.CollectionInterval)
	if err != nil || interval < time.Second || interval > 5*time.Minute {
		return Profile{}, fmt.Errorf("profile %q collection_interval must be between 1s and 5m", record.ID)
	}
	if record.MatchLabels[LabelMonitoringEnabled] != "true" || record.MatchLabels[LabelMonitoringProfile] != record.ID {
		return Profile{}, fmt.Errorf("profile %q must match its explicit Docker monitoring opt-in labels", record.ID)
	}
	profile := Profile{
		ID: record.ID, Provider: record.Provider, MatchLabels: record.MatchLabels,
		RequiredCapabilities: record.RequiredCapabilities, ReceiverType: record.ReceiverType,
		CollectionInterval: record.CollectionInterval,
		EndpointStrategies: profileEndpointStrategies(record.ID, record.ReceiverType, record.TargetPort, record.TargetPortLabel),
		Settings:           ReceiverSettings{MetricsPath: record.MetricsPath, UsernameEnv: record.UsernameEnv, PasswordEnv: record.PasswordEnv, TLSCAFile: record.TLSCAFile},
	}
	switch record.ReceiverType {
	case "redis":
		if len(profile.RequiredCapabilities) != 1 || profile.RequiredCapabilities[0] != RedisCapability {
			return Profile{}, fmt.Errorf("profile %q must declare the Redis capability", record.ID)
		}
	case "postgresql":
		if record.UsernameEnv == "" || record.PasswordEnv == "" || !environmentNamePattern.MatchString(record.UsernameEnv) || !environmentNamePattern.MatchString(record.PasswordEnv) {
			return Profile{}, fmt.Errorf("profile %q requires valid username_env and password_env references", record.ID)
		}
		if len(profile.RequiredCapabilities) != 1 || profile.RequiredCapabilities[0] != PostgresqlCapability {
			return Profile{}, fmt.Errorf("profile %q must declare the PostgreSQL capability", record.ID)
		}
	case "prometheus":
		if record.MetricsPath == "" {
			profile.Settings.MetricsPath = "/metrics"
		}
		if !strings.HasPrefix(profile.Settings.MetricsPath, "/") || len(profile.Settings.MetricsPath) > 256 || strings.ContainsAny(profile.Settings.MetricsPath, "?#\x00\r\n") {
			return Profile{}, fmt.Errorf("profile %q metrics_path must be a local path without query or fragment", record.ID)
		}
		if len(profile.RequiredCapabilities) != 1 || profile.RequiredCapabilities[0] != PrometheusCapability {
			return Profile{}, fmt.Errorf("profile %q must declare the Prometheus capability", record.ID)
		}
	default:
		return Profile{}, fmt.Errorf("profile %q uses unsupported receiver type %q", record.ID, record.ReceiverType)
	}
	return profile, nil
}

func profileEndpointStrategies(profileID, receiverType string, targetPort uint16, targetPortLabel string) []EndpointStrategy {
	alternateID := "alternate_port"
	containerID := "container_endpoint"
	if profileID != "redis" {
		alternateID = profileID + "_alternate_port"
		containerID = profileID + "_container_endpoint"
	}
	endpointFor := func(scope string) string {
		switch receiverType {
		case "redis":
			if scope == "host" {
				return "127.0.0.1:`alternate_port`"
			}
			return "`host`:`port`"
		case "postgresql":
			if scope == "host" {
				return "127.0.0.1:`alternate_port`"
			}
			return "`host`:`port`"
		default:
			if scope == "host" {
				return "127.0.0.1:`alternate_port`"
			}
			return "`joinHostPort(host, port)`"
		}
	}
	return []EndpointStrategy{
		{ID: alternateID, OS: "windows", When: "alternate_port > 0", Endpoint: endpointFor("host"), Scope: discovery.EndpointHost, Provenance: "published_port", TargetPort: targetPort, TargetPortLabel: targetPortLabel, Priority: 100},
		{ID: alternateID, OS: "linux", When: "alternate_port > 0", Endpoint: endpointFor("host"), Scope: discovery.EndpointHost, Provenance: "published_port", TargetPort: targetPort, TargetPortLabel: targetPortLabel, Priority: 100},
		{ID: containerID, OS: "linux", When: "alternate_port == 0 and host != \"\"", Endpoint: endpointFor("container"), Scope: discovery.EndpointContainer, Provenance: "container_network", TargetPort: targetPort, TargetPortLabel: targetPortLabel, Priority: 50},
	}
}
