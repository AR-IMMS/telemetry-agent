package inventory

import (
	"github.com/ar-imms/telemetry-agent/internal/inventory/observation/application"
	"github.com/ar-imms/telemetry-agent/internal/inventory/observation/domain"
)

// ErrIncompleteSnapshot is retained for callers of the compatibility package.
var ErrIncompleteSnapshot = domain.ErrIncompleteSnapshot

// ErrSnapshotConflict is retained for callers of the compatibility package.
var ErrSnapshotConflict = application.ErrSnapshotConflict

// ProviderID is the provider-neutral identifier alias retained for compatibility.
type ProviderID = domain.ProviderID

// ProviderInstanceID identifies one configured provider instance.
type ProviderInstanceID = domain.ProviderInstanceID

// ResourceKind is a provider-neutral resource class.
type ResourceKind = domain.ResourceKind

// Presence describes inventory presence derived from complete observations.
type Presence = domain.Presence

// EndpointScope identifies where an endpoint address applies.
type EndpointScope = domain.EndpointScope

// Endpoint records endpoint evidence without asserting reachability.
type Endpoint = domain.Endpoint

// ResourceKey is a provider-scoped stable identity.
type ResourceKey = domain.ResourceKey

// ResourceObservation is a provider-neutral resource observation.
type ResourceObservation = domain.ResourceObservation

// ProviderSnapshot is one provider instance's observation.
type ProviderSnapshot = domain.ProviderSnapshot

// Resource is a persisted inventory record.
type Resource = domain.Resource

// ProviderStatus is safe operational evidence for a provider instance.
type ProviderStatus = domain.ProviderStatus

// InventorySnapshot is a consistent inventory revision and its records.
type InventorySnapshot = domain.InventorySnapshot

// InventoryRepository is the inventory persistence port.
type InventoryRepository = application.InventoryRepository

// ProviderStatusReader reads bounded provider-status diagnostics.
type ProviderStatusReader = application.ProviderStatusReader

// DiscoveryProvider returns provider snapshots.
type DiscoveryProvider = application.DiscoveryProvider

// Policy defines thresholds for materializing successful absence observations.
type Policy = domain.Policy

const (
	// ResourceContainer is a container resource kind.
	ResourceContainer = domain.ResourceContainer
	// ResourceProcess is a process resource kind.
	ResourceProcess = domain.ResourceProcess
	// ResourceService is an operating-system service resource kind.
	ResourceService = domain.ResourceService
	// ResourceVM is a virtual-machine resource kind.
	ResourceVM = domain.ResourceVM
)

const (
	// PresencePresent means a complete snapshot contains the resource.
	PresencePresent = domain.PresencePresent
	// PresenceMissing means complete snapshots crossed the absence threshold.
	PresenceMissing = domain.PresenceMissing
)

const (
	// EndpointContainer identifies an address scoped to a container network.
	EndpointContainer = domain.EndpointContainer
	// EndpointHost identifies an address scoped to the host.
	EndpointHost = domain.EndpointHost
)

// MaxProviderStatusReadLimit caps provider-status-only diagnostic reads.
const MaxProviderStatusReadLimit = application.MaxProviderStatusReadLimit

// DefaultPolicy returns the current inventory absence and freshness thresholds.
func DefaultPolicy() Policy { return domain.DefaultPolicy() }

// NormalizeSnapshot validates and deterministically copies provider evidence.
func NormalizeSnapshot(snapshot ProviderSnapshot) (ProviderSnapshot, error) {
	return domain.NormalizeSnapshot(snapshot)
}

// ReconcileProviderSnapshot applies complete evidence to one provider's records.
func ReconcileProviderSnapshot(current []Resource, snapshot ProviderSnapshot, policy Policy) ([]Resource, bool, error) {
	return domain.ReconcileProviderSnapshot(current, snapshot, policy)
}
