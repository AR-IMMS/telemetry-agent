package discovery

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode"
)

const (
	maxResources      = 10000
	maxAttributes     = 64
	maxAttributeKey   = 128
	maxAttributeValue = 1024
	maxEndpoints      = 256
	maxDisplayName    = 256
)

// NormalizeSnapshot validates a provider snapshot and returns a defensive,
// deterministically ordered copy. An incomplete snapshot is fail-closed.
func NormalizeSnapshot(snapshot ProviderSnapshot) (ProviderSnapshot, error) {
	if !snapshot.Complete {
		return ProviderSnapshot{}, ErrIncompleteSnapshot
	}
	if !safeIdentifier.MatchString(string(snapshot.ProviderID)) {
		return ProviderSnapshot{}, fmt.Errorf("invalid provider ID")
	}
	instance := string(snapshot.ProviderInstanceID)
	if instance == "" || len(instance) > 128 || strings.TrimSpace(instance) != instance || strings.ContainsAny(instance, "/\\") || containsControl(instance) {
		return ProviderSnapshot{}, fmt.Errorf("invalid provider instance ID")
	}
	if snapshot.ObservedAt.IsZero() {
		return ProviderSnapshot{}, fmt.Errorf("provider observation time is required")
	}
	if len(snapshot.Resources) > maxResources {
		return ProviderSnapshot{}, fmt.Errorf("provider resource limit exceeded")
	}
	result := snapshot
	result.Resources = make([]ResourceObservation, 0, len(snapshot.Resources))
	seen := make(map[string]ResourceObservation, len(snapshot.Resources))
	for _, raw := range snapshot.Resources {
		observation, err := normalizeObservation(raw)
		if err != nil {
			return ProviderSnapshot{}, err
		}
		if previous, ok := seen[observation.ProviderResourceID]; ok {
			if !reflect.DeepEqual(previous, observation) {
				return ProviderSnapshot{}, fmt.Errorf("conflicting duplicate provider resource identity")
			}
			continue
		}
		seen[observation.ProviderResourceID] = observation
		result.Resources = append(result.Resources, observation)
	}
	sort.Slice(result.Resources, func(i, j int) bool {
		return result.Resources[i].ProviderResourceID < result.Resources[j].ProviderResourceID
	})
	return result, nil
}

func normalizeObservation(raw ResourceObservation) (ResourceObservation, error) {
	id := raw.ProviderResourceID
	if id == "" || len(id) > 256 || strings.TrimSpace(id) != id || strings.ContainsAny(id, "/\\") || containsControl(id) {
		return ResourceObservation{}, fmt.Errorf("invalid provider resource ID")
	}
	switch raw.Kind {
	case ResourceContainer, ResourceProcess, ResourceService, ResourceVM:
	default:
		return ResourceObservation{}, fmt.Errorf("unsupported resource kind %q", raw.Kind)
	}
	if len(raw.DisplayName) > maxDisplayName || len(raw.LifecycleState) > 64 || containsControl(raw.DisplayName) || containsControl(raw.LifecycleState) {
		return ResourceObservation{}, fmt.Errorf("resource display or lifecycle field is invalid")
	}
	result := raw
	result.Attributes = make(map[string]string, len(raw.Attributes))
	if len(raw.Attributes) > maxAttributes {
		return ResourceObservation{}, fmt.Errorf("resource attribute limit exceeded")
	}
	for key, value := range raw.Attributes {
		if key == "" || len(key) > maxAttributeKey || len(value) > maxAttributeValue || sensitiveAttributeKey(key) || containsControl(key) || containsControl(value) {
			return ResourceObservation{}, fmt.Errorf("resource attribute is invalid")
		}
		result.Attributes[key] = value
	}
	result.Endpoints = append([]Endpoint(nil), raw.Endpoints...)
	if len(result.Endpoints) > maxEndpoints {
		return ResourceObservation{}, fmt.Errorf("resource endpoint limit exceeded")
	}
	for _, endpoint := range result.Endpoints {
		if endpoint.Port == 0 && endpoint.Provenance != "network_address" {
			return ResourceObservation{}, fmt.Errorf("endpoint port must be positive")
		}
		if endpoint.Address == "" && endpoint.Provenance != "exposed_port" {
			return ResourceObservation{}, fmt.Errorf("endpoint address is required")
		}
		if len(endpoint.Address) > 256 || len(endpoint.Provenance) == 0 || len(endpoint.Provenance) > 64 || containsControl(endpoint.Address) || containsControl(endpoint.Provenance) {
			return ResourceObservation{}, fmt.Errorf("endpoint field is invalid")
		}
		if endpoint.Scope != EndpointContainer && endpoint.Scope != EndpointHost {
			return ResourceObservation{}, fmt.Errorf("invalid endpoint scope")
		}
		switch endpoint.Protocol {
		case "", "tcp", "udp", "sctp":
		default:
			return ResourceObservation{}, fmt.Errorf("invalid endpoint protocol")
		}
	}
	sort.Slice(result.Endpoints, func(i, j int) bool {
		a, b := result.Endpoints[i], result.Endpoints[j]
		return strings.Join([]string{string(a.Scope), a.Address, fmt.Sprint(a.Port), a.Protocol, a.Provenance}, "\x00") <
			strings.Join([]string{string(b.Scope), b.Address, fmt.Sprint(b.Port), b.Protocol, b.Provenance}, "\x00")
	})
	return result, nil
}

// FreshnessAt reports whether the last evidence is within the supplied bound.
func (r Resource) FreshnessAt(now time.Time, staleAfter time.Duration) string {
	if staleAfter <= 0 || r.LastSeenAt.IsZero() || now.Sub(r.LastSeenAt) > staleAfter {
		return "stale"
	}
	return "fresh"
}

func containsControl(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func sensitiveAttributeKey(key string) bool {
	normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "", ".", "", " ", "").Replace(key))
	for _, marker := range []string{"password", "secret", "token", "credential", "authorization", "privatekey", "apikey", "environment", "command", "volume", "mount"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return normalized == "env" || normalized == "args"
}
