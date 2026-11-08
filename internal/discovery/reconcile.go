package discovery

import (
	"fmt"
	"reflect"
	"sort"
	"time"
)

// Policy defines materialization thresholds for successful absences.
type Policy struct {
	MissingAfterSnapshots uint32
	StaleAfter            time.Duration
}

// DefaultPolicy requires two complete misses and marks evidence stale after two minutes.
func DefaultPolicy() Policy {
	return Policy{MissingAfterSnapshots: 2, StaleAfter: 2 * time.Minute}
}

// ReconcileProviderSnapshot applies one complete snapshot to that provider's records.
// A provider error must be handled separately and can never mark a resource missing.
func ReconcileProviderSnapshot(current []Resource, snapshot ProviderSnapshot, policy Policy) ([]Resource, bool, error) {
	normalized, err := NormalizeSnapshot(snapshot)
	if err != nil {
		return nil, false, err
	}
	if policy.MissingAfterSnapshots == 0 {
		return nil, false, fmt.Errorf("missing snapshot threshold must be positive")
	}
	observed := make(map[string]struct{}, len(normalized.Resources))
	for _, item := range normalized.Resources {
		observed[item.ProviderResourceID] = struct{}{}
	}
	byID := make(map[string]Resource, len(current))
	for _, resource := range current {
		if resource.Key.Provider != normalized.ProviderID || resource.Key.ProviderInstance != normalized.ProviderInstanceID {
			continue
		}
		if err := resource.Key.Validate(); err != nil {
			return nil, false, err
		}
		if _, exists := byID[resource.Key.ProviderResourceID]; exists {
			return nil, false, fmt.Errorf("duplicate persisted inventory resource")
		}
		byID[resource.Key.ProviderResourceID] = cloneResource(resource)
	}
	next := make(map[string]Resource, len(byID)+len(normalized.Resources))
	for id, old := range byID {
		next[id] = old
	}
	changed := false
	for _, item := range normalized.Resources {
		id := item.ProviderResourceID
		key := ResourceKey{Provider: normalized.ProviderID, ProviderInstance: normalized.ProviderInstanceID, ProviderResourceID: id}
		old, exists := next[id]
		resource := Resource{
			Key: key, Kind: item.Kind, DisplayName: item.DisplayName, LifecycleState: item.LifecycleState,
			Attributes: cloneMap(item.Attributes), Endpoints: append([]Endpoint(nil), item.Endpoints...),
			Presence: PresencePresent, FirstSeenAt: normalized.ObservedAt, LastSeenAt: normalized.ObservedAt,
		}
		if exists {
			resource.FirstSeenAt = old.FirstSeenAt
			if !materiallyEqual(old, resource) || old.Presence != PresencePresent {
				changed = true
			}
		} else {
			changed = true
		}
		next[id] = resource
	}
	for id, old := range next {
		if _, exists := observed[id]; exists {
			continue
		}
		if old.Presence == PresencePresent {
			old.MissingSnapshots++
			if old.MissingSnapshots >= policy.MissingAfterSnapshots {
				old.Presence = PresenceMissing
				when := normalized.ObservedAt
				old.MissingSince = &when
				changed = true
			}
		}
		next[id] = old
	}
	result := make([]Resource, 0, len(next))
	for _, resource := range next {
		result = append(result, resource)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Key.String() < result[j].Key.String() })
	return result, changed, nil
}

func materiallyEqual(a, b Resource) bool {
	return a.Key == b.Key && a.Kind == b.Kind && a.DisplayName == b.DisplayName &&
		a.LifecycleState == b.LifecycleState && reflect.DeepEqual(a.Attributes, b.Attributes) &&
		reflect.DeepEqual(a.Endpoints, b.Endpoints)
}

func cloneResource(resource Resource) Resource {
	resource.Attributes = cloneMap(resource.Attributes)
	resource.Endpoints = append([]Endpoint(nil), resource.Endpoints...)
	if resource.MissingSince != nil {
		value := *resource.MissingSince
		resource.MissingSince = &value
	}
	return resource
}

func cloneMap(input map[string]string) map[string]string {
	if input == nil {
		return map[string]string{}
	}
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}
