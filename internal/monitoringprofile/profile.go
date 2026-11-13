package monitoringprofile

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/discovery"
)

const (
	// LabelMonitoringEnabled opts one Docker workload into Agent monitoring.
	LabelMonitoringEnabled = "ar-imms.monitoring.enabled"
	// LabelMonitoringProfile selects one reviewed monitoring profile.
	LabelMonitoringProfile = "ar-imms.monitoring.profile"
	// LabelMonitoringPort optionally overrides a workload's profile default TCP port.
	LabelMonitoringPort = "ar-imms.monitoring.port"
	// LabelWorkloadID is an optional stable logical identity supplied by the workload owner.
	LabelWorkloadID = "ar-imms.workload.id"

	// RedisCapability identifies the pinned Collector Redis metrics receiver.
	RedisCapability = "otel.receiver.redis.metrics"
	// PostgresqlCapability identifies the pinned Collector PostgreSQL metrics receiver.
	PostgresqlCapability = "otel.receiver.postgresql.metrics"
	// PrometheusCapability identifies the pinned Collector Prometheus metrics receiver.
	PrometheusCapability = "otel.receiver.prometheus.metrics"
)

var profileIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
var targetPortLabelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.\/-]{0,127}$`)

// EndpointStrategy describes one platform-specific endpoint selection rule.
type EndpointStrategy struct {
	ID         string
	OS         string
	When       string
	Endpoint   string
	Scope      discovery.EndpointScope
	Provenance string
	TargetPort uint16
	// TargetPortLabel optionally supplies a validated private TCP port override.
	TargetPortLabel string
	// PrivatePortVariable names the Observer variable that retains the workload port.
	PrivatePortVariable string
	Priority            int
}

// Profile defines a reviewed provider match and native Collector receiver rule.
type Profile struct {
	ID                   string
	Provider             string
	MatchLabels          map[string]string
	RequiredCapabilities []string
	ReceiverType         string
	CollectionInterval   string
	EndpointStrategies   []EndpointStrategy
	Settings             ReceiverSettings
}

// ReceiverSettings contains the deliberately small set of typed settings
// supported by the built-in native receiver adapters.
type ReceiverSettings struct {
	MetricsPath string
	UsernameEnv string
	PasswordEnv string
	TLSCAFile   string
}

// Registry contains validated built-in monitoring profiles.
type Registry struct{ profiles []Profile }

// RedisProfile returns the initial Docker Redis profile and endpoint strategies.
func RedisProfile() Profile {
	return Profile{
		ID:       "redis",
		Provider: "docker",
		MatchLabels: map[string]string{
			LabelMonitoringEnabled: "true",
			LabelMonitoringProfile: "redis",
		},
		RequiredCapabilities: []string{RedisCapability},
		ReceiverType:         "redis",
		CollectionInterval:   "10s",
		EndpointStrategies: []EndpointStrategy{
			{ID: "alternate_port", OS: "windows", When: "alternate_port > 0", Endpoint: "127.0.0.1:`alternate_port`", Scope: discovery.EndpointHost, Provenance: "published_port", TargetPort: 6379, TargetPortLabel: LabelMonitoringPort, PrivatePortVariable: "port", Priority: 100},
			{ID: "alternate_port", OS: "linux", When: "alternate_port > 0", Endpoint: "`host`:`port`", Scope: discovery.EndpointHost, Provenance: "published_port", TargetPort: 6379, TargetPortLabel: LabelMonitoringPort, PrivatePortVariable: "alternate_port", Priority: 100},
			{ID: "container_endpoint", OS: "linux", When: "alternate_port == 0 and host != \"\"", Endpoint: "`host`:`port`", Scope: discovery.EndpointContainer, Provenance: "container_network", TargetPort: 6379, TargetPortLabel: LabelMonitoringPort, PrivatePortVariable: "port", Priority: 50},
		},
	}
}

// NewRegistry validates and copies profiles so callers cannot mutate registry policy.
func NewRegistry(profiles ...Profile) (Registry, error) {
	if len(profiles) == 0 || len(profiles) > 128 {
		return Registry{}, fmt.Errorf("monitoring profile count must be between 1 and 128")
	}
	ordered := make([]Profile, 0, len(profiles))
	seen := make(map[string]struct{}, len(profiles))
	seenRules := make(map[string]struct{})
	for _, profile := range profiles {
		if !profileIDPattern.MatchString(profile.ID) || profile.Provider == "" || profile.ReceiverType == "" || profile.CollectionInterval == "" ||
			len(profile.MatchLabels) == 0 || len(profile.RequiredCapabilities) == 0 || len(profile.EndpointStrategies) == 0 {
			return Registry{}, fmt.Errorf("monitoring profile is incomplete")
		}
		if _, exists := seen[profile.Provider+"/"+profile.ID]; exists {
			return Registry{}, fmt.Errorf("duplicate monitoring profile identity")
		}
		seen[profile.Provider+"/"+profile.ID] = struct{}{}
		copyOfProfile := cloneProfile(profile)
		if err := validateProfile(copyOfProfile); err != nil {
			return Registry{}, err
		}
		for _, strategy := range copyOfProfile.EndpointStrategies {
			ruleKey := copyOfProfile.Provider + "/" + strategy.OS + "/" + copyOfProfile.ReceiverType + "/" + strategy.ID
			if _, exists := seenRules[ruleKey]; exists {
				return Registry{}, fmt.Errorf("monitoring profiles conflict on Receiver Creator rule %q", ruleKey)
			}
			seenRules[ruleKey] = struct{}{}
		}
		ordered = append(ordered, copyOfProfile)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Provider == ordered[j].Provider {
			return ordered[i].ID < ordered[j].ID
		}
		return ordered[i].Provider < ordered[j].Provider
	})
	return Registry{profiles: ordered}, nil
}

// DefaultRegistry returns the initial reviewed Docker/Redis profile registry.
func DefaultRegistry() Registry {
	registry, _ := NewRegistry(RedisProfile())
	return registry
}

// Profiles returns defensive copies of every registered profile in stable order.
func (r Registry) Profiles() []Profile {
	profiles := make([]Profile, 0, len(r.profiles))
	for _, profile := range r.profiles {
		profiles = append(profiles, cloneProfile(profile))
	}
	return profiles
}

// Match returns a copy of the profile selected by exact provider and labels.
func (r Registry) Match(provider string, attributes map[string]string) (Profile, bool) {
	for _, profile := range r.profiles {
		if profile.Provider != provider {
			continue
		}
		matched := true
		for key, expected := range profile.MatchLabels {
			if attributes["label."+key] != expected {
				matched = false
				break
			}
		}
		if matched {
			return cloneProfile(profile), true
		}
	}
	return Profile{}, false
}

func validateProfile(profile Profile) error {
	if len(profile.RequiredCapabilities) != 1 {
		return fmt.Errorf("monitoring profile %q must declare exactly one supported Collector capability", profile.ID)
	}
	expectedCapability := map[string]string{
		"redis": RedisCapability, "postgresql": PostgresqlCapability, "prometheus": PrometheusCapability,
	}[profile.ReceiverType]
	if expectedCapability == "" || profile.RequiredCapabilities[0] != expectedCapability {
		return fmt.Errorf("monitoring profile %q has an unsupported receiver/capability combination", profile.ID)
	}
	if profile.ReceiverType == "postgresql" && (!environmentNamePattern.MatchString(profile.Settings.UsernameEnv) || !environmentNamePattern.MatchString(profile.Settings.PasswordEnv)) {
		return fmt.Errorf("monitoring profile %q requires valid PostgreSQL credential environment references", profile.ID)
	}
	if profile.ReceiverType != "postgresql" && profile.Settings.TLSCAFile != "" {
		return fmt.Errorf("monitoring profile %q uses PostgreSQL TLS settings with a non-PostgreSQL receiver", profile.ID)
	}
	if profile.Settings.TLSCAFile != "" && (!filepath.IsAbs(profile.Settings.TLSCAFile) || strings.ContainsAny(profile.Settings.TLSCAFile, "\x00\r\n")) {
		return fmt.Errorf("monitoring profile %q requires an absolute, valid PostgreSQL TLS CA file path", profile.ID)
	}
	if profile.ReceiverType == "prometheus" && (profile.Settings.MetricsPath == "" || !strings.HasPrefix(profile.Settings.MetricsPath, "/") || len(profile.Settings.MetricsPath) > 256 || strings.ContainsAny(profile.Settings.MetricsPath, "?#\x00\r\n")) {
		return fmt.Errorf("monitoring profile %q has an invalid Prometheus metrics path", profile.ID)
	}
	for key, value := range profile.MatchLabels {
		if strings.TrimSpace(key) != key || key == "" || strings.ContainsAny(key+value, "\x00\r\n") {
			return fmt.Errorf("monitoring profile %q has an invalid label match", profile.ID)
		}
	}
	seen := make(map[string]struct{}, len(profile.EndpointStrategies))
	for _, strategy := range profile.EndpointStrategies {
		if strategy.ID == "" || (strategy.OS != "linux" && strategy.OS != "windows") || strategy.When == "" || strategy.Endpoint == "" || strategy.Priority <= 0 || strategy.Provenance == "" || (strategy.Scope != discovery.EndpointContainer && strategy.Scope != discovery.EndpointHost) || (strategy.PrivatePortVariable != "" && strategy.PrivatePortVariable != "port" && strategy.PrivatePortVariable != "alternate_port") || (strategy.TargetPortLabel != "" && (strategy.TargetPort == 0 || !targetPortLabelPattern.MatchString(strategy.TargetPortLabel))) {
			return fmt.Errorf("monitoring profile %q has an invalid endpoint strategy", profile.ID)
		}
		key := strategy.OS + "/" + strategy.ID
		if _, exists := seen[key]; exists {
			return fmt.Errorf("monitoring profile %q has duplicate endpoint strategy", profile.ID)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func cloneProfile(profile Profile) Profile {
	clone := profile
	clone.MatchLabels = make(map[string]string, len(profile.MatchLabels))
	for key, value := range profile.MatchLabels {
		clone.MatchLabels[key] = value
	}
	clone.RequiredCapabilities = append([]string(nil), profile.RequiredCapabilities...)
	clone.EndpointStrategies = append([]EndpointStrategy(nil), profile.EndpointStrategies...)
	return clone
}

// CredentialsConfigured reports whether all local credential references needed
// by a profile resolve to non-empty values. It never returns or logs a value.
func CredentialsConfigured(profile Profile) bool {
	if profile.ReceiverType != "postgresql" {
		return true
	}
	username, usernameOK := os.LookupEnv(profile.Settings.UsernameEnv)
	password, passwordOK := os.LookupEnv(profile.Settings.PasswordEnv)
	return usernameOK && strings.TrimSpace(username) != "" && passwordOK && strings.TrimSpace(password) != ""
}
