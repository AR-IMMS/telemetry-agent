package application

import (
	"sort"
	"time"

	inventorydomain "github.com/ar-imms/telemetry-agent/internal/inventory/observation/domain"
)

// ProjectInventory maps one persisted Inventory snapshot to the stable Monitoring match contract.
func ProjectInventory(snapshot inventorydomain.InventorySnapshot, asOf time.Time) MatchSnapshot {
	resources := make([]MatchResource, 0, len(snapshot.Resources))
	for _, resource := range snapshot.Resources {
		attributes := make(map[string]string, len(resource.Attributes))
		for key, value := range resource.Attributes {
			attributes[key] = value
		}
		endpoints := make([]MatchEndpoint, 0, len(resource.Endpoints))
		for _, endpoint := range resource.Endpoints {
			endpoints = append(endpoints, MatchEndpoint{
				Address: endpoint.Address, Port: endpoint.Port, Protocol: endpoint.Protocol,
				Scope: string(endpoint.Scope), Provenance: endpoint.Provenance,
			})
		}
		resources = append(resources, MatchResource{
			Provider: string(resource.Key.Provider), ProviderInstance: string(resource.Key.ProviderInstance),
			ResourceID: resource.Key.ProviderResourceID, Kind: string(resource.Kind),
			LifecycleState: resource.LifecycleState, Presence: string(resource.Presence),
			Attributes: attributes, Endpoints: endpoints, LastSeenAt: resource.LastSeenAt,
		})
	}
	sort.Slice(resources, func(i, j int) bool {
		if resources[i].Provider != resources[j].Provider {
			return resources[i].Provider < resources[j].Provider
		}
		if resources[i].ProviderInstance != resources[j].ProviderInstance {
			return resources[i].ProviderInstance < resources[j].ProviderInstance
		}
		return resources[i].ResourceID < resources[j].ResourceID
	})
	return MatchSnapshot{
		InventoryRevision: snapshot.Revision, AsOf: asOf,
		StaleAfter: inventorydomain.DefaultPolicy().StaleAfter, Resources: resources,
	}
}
