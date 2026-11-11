package redis

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/integration"
)

const (
	definitionID           = "redis"
	definitionVersion      = "1.0.0"
	redisMetricsCapability = "otel.receiver.redis.metrics"
)

// Definition returns the versioned Redis metrics definition registered by the
// Agent composition root on platforms with a verified Redis receiver.
func Definition() integration.IntegrationDefinition {
	return integration.IntegrationDefinition{
		ID:       definitionID,
		Version:  definitionVersion,
		Validate: validateConfiguration,
		TemplateSettingRules: map[string]integration.TemplateSettingRule{
			"endpoint":            {AllowResourceBinding: true},
			"collection_interval": {Default: "10s", DefaultOverridable: true},
		},
		Matching: &integration.MatchingDefinition{
			Selector: &integration.MatchSelector{
				ResourceKinds: []string{"container"}, LifecycleStates: []string{"running"},
				ExactAttributes: map[string]string{"container.image.repository": "docker.io/library/redis"},
			},
			BuildCandidate:       buildDiscoveredCandidate,
			RequiredCapabilities: []string{redisMetricsCapability},
		},
		Contribute: func(instance integration.IntegrationInstance) (integration.Contribution, error) {
			if err := integration.ValidateInstance(instance); err != nil {
				return integration.Contribution{}, err
			}
			if err := validateConfiguration(instance.Configuration); err != nil {
				return integration.Contribution{}, fmt.Errorf("validate Redis settings: %w", err)
			}
			componentConfig := map[string]any{
				"endpoint": instance.Configuration["endpoint"],
				// The receiver disables these identifying resource attributes by
				// default. Enable them so multiple instances remain distinguishable.
				"resource_attributes": map[string]any{
					"server.address": map[string]any{"enabled": true},
					"server.port":    map[string]any{"enabled": true},
				},
			}
			if interval, exists := instance.Configuration["collection_interval"]; exists {
				componentConfig["collection_interval"] = interval
			}
			intent, err := integration.NewComponentIntent(redisMetricsCapability, componentConfig)
			if err != nil {
				return integration.Contribution{}, err
			}
			return integration.NewContribution([]integration.ComponentIntent{intent}), nil
		},
	}
}

func buildDiscoveredCandidate(resource integration.MatchResource) (integration.CandidateBuild, error) {
	endpoints := make(map[string]integration.MatchEndpoint)
	for _, endpoint := range resource.Endpoints {
		if endpoint.Scope != "host" || endpoint.Provenance != "published_port" || endpoint.Protocol != "tcp" || endpoint.Port == 0 {
			continue
		}
		address, err := netip.ParseAddr(endpoint.Address)
		if err != nil || !address.IsLoopback() {
			continue
		}
		key := net.JoinHostPort(address.String(), strconv.Itoa(int(endpoint.Port)))
		endpoints[key] = endpoint
	}
	if len(endpoints) != 1 {
		return integration.CandidateBuild{
			RequiredInputs: []integration.RequiredInput{{Name: "endpoint", Reason: "a unique loopback host-published TCP endpoint is required"}},
		}, nil
	}
	for endpoint := range endpoints {
		return integration.CandidateBuild{
			Settings: map[string]any{"endpoint": endpoint},
			Evidence: []integration.MatchEvidence{{Code: "endpoint_selected", Source: "host/published_port/tcp", Value: endpoint}},
		}, nil
	}
	return integration.CandidateBuild{}, fmt.Errorf("no usable endpoint")
}

func validateConfiguration(configuration map[string]any) error {
	if len(configuration) < 1 || len(configuration) > 2 {
		return fmt.Errorf("Redis settings require endpoint and optional collection_interval")
	}
	endpoint, ok := configuration["endpoint"].(string)
	if !ok || endpoint == "" || strings.TrimSpace(endpoint) != endpoint || strings.ContainsAny(endpoint, "/?#@") {
		return fmt.Errorf("Redis endpoint must be host:port without URL or credential syntax")
	}
	host, portText, err := net.SplitHostPort(endpoint)
	if err != nil || !validHost(host) {
		return fmt.Errorf("Redis endpoint must be a valid host:port")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("Redis endpoint port must be between 1 and 65535")
	}
	if intervalValue, exists := configuration["collection_interval"]; exists {
		intervalText, ok := intervalValue.(string)
		interval, parseErr := time.ParseDuration(intervalText)
		if !ok || parseErr != nil || interval <= 0 {
			return fmt.Errorf("Redis collection_interval must be a positive duration")
		}
	}
	for key := range configuration {
		if key != "endpoint" && key != "collection_interval" {
			return fmt.Errorf("unsupported Redis setting %q", key)
		}
	}
	return nil
}

func validHost(host string) bool {
	if host == "" || strings.TrimSpace(host) != host {
		return false
	}
	if net.ParseIP(host) != nil {
		return true
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
				(char >= '0' && char <= '9') || char == '-' {
				continue
			}
			return false
		}
	}
	return true
}
