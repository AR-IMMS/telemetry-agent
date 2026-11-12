package monitoringprofile

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/config"
)

// DockerCollectorLayer projects registered Docker profiles into one
// docker_observer extension and a receiver_creator rule set. The returned
// receiver names must be added to the metrics pipeline by the composition root.
func (r Registry) DockerCollectorLayer(osName, endpoint, nodeID string) (config.InlineLayer, []string, error) {
	if osName != "linux" && osName != "windows" {
		return config.InlineLayer{}, nil, fmt.Errorf("Docker monitoring is unsupported on this platform")
	}
	if err := validateDockerEndpoint(osName, endpoint); err != nil {
		return config.InlineLayer{}, nil, err
	}
	if strings.TrimSpace(nodeID) == "" || len(nodeID) > 128 || strings.ContainsAny(nodeID, "\x00\r\n") {
		return config.InlineLayer{}, nil, fmt.Errorf("Core node identity is required for Docker monitoring")
	}
	profiles := make([]any, 0, len(r.profiles))
	for _, profile := range r.profiles {
		if profile.Provider != "docker" {
			continue
		}
		receivers := make(map[string]any)
		for _, strategy := range profile.EndpointStrategies {
			if strategy.OS != osName {
				continue
			}
			id := profile.ReceiverType + "/" + strategy.ID
			if _, duplicate := receivers[id]; duplicate {
				return config.InlineLayer{}, nil, fmt.Errorf("duplicate receiver rule %q", id)
			}
			receivers[id] = map[string]any{
				"rule":   dockerProfileRule(profile) + " and (" + strategy.When + ") and (" + targetPortRule(strategy) + ")",
				"config": map[string]any{"endpoint": strategy.Endpoint, "collection_interval": profile.CollectionInterval},
				"resource_attributes": map[string]any{
					"node_id":          nodeID,
					"resource_id":      "`container_id`",
					"workload_id":      "`labels[\"ar-imms.workload.id\"]`",
					"container.name":   "`name`",
					"container.id":     "`container_id`",
					"server.address":   "`host`",
					"server.port":      "`port`",
					"ar_imms.provider": profile.Provider,
					"service.name":     profile.ID,
				},
			}
		}
		if len(receivers) > 0 {
			profiles = append(profiles, receivers)
		}
	}
	if len(profiles) == 0 {
		return config.InlineLayer{}, nil, fmt.Errorf("no Docker monitoring profile supports %s", osName)
	}
	mergedReceivers := make(map[string]any)
	for _, value := range profiles {
		for id, receiver := range value.(map[string]any) {
			mergedReceivers[id] = receiver
		}
	}
	layer := config.InlineLayer{
		Name: "docker_monitoring_profiles",
		Document: map[string]any{
			"extensions": map[string]any{"docker_observer": map[string]any{"endpoint": endpoint, "timeout": "5s", "api_version": "1.44", "use_hostname_if_present": false}},
			"receivers": map[string]any{"receiver_creator": map[string]any{
				"watch_observers": []any{"docker_observer"},
				"receivers":       mergedReceivers,
			}},
			"service": map[string]any{"extensions": []any{"health_check", "docker_observer"}},
		},
	}
	return layer, []string{"receiver_creator"}, nil
}

func targetPortRule(strategy EndpointStrategy) string {
	if strategy.TargetPortLabel == "" {
		if strategy.TargetPort == 0 {
			return "true"
		}
		return fmt.Sprintf("port == %d", strategy.TargetPort)
	}
	label := fmt.Sprintf("labels[%q]", strategy.TargetPortLabel)
	return fmt.Sprintf("(%s == \"\" and port == %d) or (%s != \"\" and port == int(%s))", label, strategy.TargetPort, label, label)
}

func dockerProfileRule(profile Profile) string {
	keys := make([]string, 0, len(profile.MatchLabels))
	for key := range profile.MatchLabels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := []string{`type == "container"`}
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf(`labels[%q] == %q`, key, profile.MatchLabels[key]))
	}
	return strings.Join(parts, " and ")
}

func validateDockerEndpoint(osName, endpoint string) error {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path == "" {
		return fmt.Errorf("explicit local Docker endpoint is required")
	}
	switch osName {
	case "linux":
		if parsed.Scheme != "unix" || parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/") {
			return fmt.Errorf("Linux Docker endpoint must be an absolute Unix socket")
		}
	case "windows":
		if parsed.Scheme != "npipe" || !strings.HasPrefix(strings.ToLower(parsed.Host+parsed.Path), "//./pipe/") {
			return fmt.Errorf("Windows Docker endpoint must use a local named pipe")
		}
	}
	return nil
}
