package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"gopkg.in/yaml.v3"
)

// NewReadinessToken returns an unpredictable value for one Collector config.
func NewReadinessToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate Collector readiness token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

// ReadinessLayer configures the existing health_check extension to return the
// candidate-specific token after it reports healthy.
func ReadinessLayer(token string) InlineLayer {
	return InlineLayer{
		Name: "generation-readiness",
		Document: map[string]any{
			"extensions": map[string]any{
				"health_check": map[string]any{
					"response_body": map[string]any{"healthy": token},
				},
			},
		},
	}
}

// ReadinessTokenFromConfig returns the configured healthy response body, or
// an empty string for legacy configurations without generation binding.
func ReadinessTokenFromConfig(content []byte) (string, error) {
	var document map[string]any
	if err := yaml.Unmarshal(content, &document); err != nil {
		return "", fmt.Errorf("parse Collector config readiness token: %w", err)
	}
	value := nestedValue(document, "extensions", "health_check", "response_body", "healthy")
	token, ok := value.(string)
	if !ok {
		return "", nil
	}
	return token, nil
}

// WithReadinessToken overlays a candidate token while preserving all other
// Collector settings in the launch artifact.
func WithReadinessToken(content []byte, token string) ([]byte, error) {
	if token == "" {
		return nil, fmt.Errorf("Collector readiness token is required")
	}
	var document map[string]any
	if err := yaml.Unmarshal(content, &document); err != nil {
		return nil, fmt.Errorf("parse Collector config for readiness overlay: %w", err)
	}
	if document == nil {
		document = map[string]any{}
	}
	merged := mergeDocument(document, ReadinessLayer(token).Document)
	updated, err := yaml.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("encode Collector config readiness overlay: %w", err)
	}
	return updated, nil
}

func nestedValue(document map[string]any, path ...string) any {
	var current any = document
	for _, part := range path {
		mapping, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = mapping[part]
	}
	return current
}
