package redis

import (
	"fmt"
	"net"
	"strconv"
	"strings"

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
		Contribute: func(instance integration.IntegrationInstance) (integration.Contribution, error) {
			if err := integration.ValidateInstance(instance); err != nil {
				return integration.Contribution{}, err
			}
			if err := validateConfiguration(instance.Configuration); err != nil {
				return integration.Contribution{}, fmt.Errorf("validate Redis settings: %w", err)
			}
			intent, err := integration.NewComponentIntent(redisMetricsCapability, map[string]any{
				"endpoint": instance.Configuration["endpoint"],
			})
			if err != nil {
				return integration.Contribution{}, err
			}
			return integration.NewContribution([]integration.ComponentIntent{intent}), nil
		},
	}
}

func validateConfiguration(configuration map[string]any) error {
	if len(configuration) != 1 {
		return fmt.Errorf("Redis settings require only endpoint")
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
