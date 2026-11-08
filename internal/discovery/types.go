package discovery

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ErrIncompleteSnapshot indicates a provider did not confirm a complete inventory.
var ErrIncompleteSnapshot = errors.New("provider snapshot is incomplete")

// ErrSnapshotConflict indicates another writer changed inventory before commit.
var ErrSnapshotConflict = errors.New("inventory snapshot changed concurrently")

var safeIdentifier = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// ProviderID identifies a discovery implementation, such as "docker".
type ProviderID string

// ProviderInstanceID identifies one explicitly configured instance of a provider.
type ProviderInstanceID string

// ResourceKind is a provider-neutral class of observed resource.
type ResourceKind string

const (
	// ResourceContainer identifies a container resource.
	ResourceContainer ResourceKind = "container"
	// ResourceProcess identifies a host process resource.
	ResourceProcess ResourceKind = "process"
	// ResourceService identifies an operating system service.
	ResourceService ResourceKind = "service"
	// ResourceVM identifies a virtual machine resource.
	ResourceVM ResourceKind = "vm"
)

// Presence describes whether a complete observation currently supports existence.
type Presence string

const (
	// PresencePresent means a complete successful snapshot contains the resource.
	PresencePresent Presence = "present"
	// PresenceMissing means complete successful snapshots crossed the absence threshold.
	PresenceMissing Presence = "missing"
)

// EndpointScope states where an endpoint address applies.
type EndpointScope string

const (
	// EndpointContainer identifies an address scoped to the container network.
	EndpointContainer EndpointScope = "container"
	// EndpointHost identifies an address scoped to the host.
	EndpointHost EndpointScope = "host"
)

// Endpoint records address evidence without asserting reachability.
type Endpoint struct {
	Address    string        `json:"address"`
	Port       uint16        `json:"port"`
	Protocol   string        `json:"protocol,omitempty"`
	Scope      EndpointScope `json:"scope"`
	Provenance string        `json:"provenance"`
}

// ResourceKey is a provider-scoped stable identity.
type ResourceKey struct {
	Provider           ProviderID         `json:"provider"`
	ProviderInstance   ProviderInstanceID `json:"provider_instance"`
	ProviderResourceID string             `json:"provider_resource_id"`
}

// String returns a stable, CLI-friendly representation of the composite key.
func (k ResourceKey) String() string {
	return string(k.Provider) + "/" + string(k.ProviderInstance) + "/" + k.ProviderResourceID
}

// Validate checks that all identity elements are bounded and well-formed.
func (k ResourceKey) Validate() error {
	if !safeIdentifier.MatchString(string(k.Provider)) {
		return fmt.Errorf("invalid provider ID")
	}
	instance := string(k.ProviderInstance)
	if instance == "" || len(instance) > 128 || strings.TrimSpace(instance) != instance {
		return fmt.Errorf("invalid provider instance ID")
	}
	if k.ProviderResourceID == "" || len(k.ProviderResourceID) > 256 || strings.TrimSpace(k.ProviderResourceID) != k.ProviderResourceID {
		return fmt.Errorf("invalid provider resource ID")
	}
	for _, r := range k.ProviderResourceID {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("invalid provider resource ID")
		}
	}
	return nil
}

// ResourceObservation is one provider's normalized view of a resource.
type ResourceObservation struct {
	ProviderResourceID string            `json:"provider_resource_id"`
	Kind               ResourceKind      `json:"kind"`
	DisplayName        string            `json:"display_name,omitempty"`
	LifecycleState     string            `json:"lifecycle_state,omitempty"`
	Attributes         map[string]string `json:"attributes,omitempty"`
	Endpoints          []Endpoint        `json:"endpoints,omitempty"`
}

// ProviderSnapshot is one complete observation of a provider instance.
type ProviderSnapshot struct {
	ProviderID         ProviderID            `json:"provider"`
	ProviderInstanceID ProviderInstanceID    `json:"provider_instance"`
	Complete           bool                  `json:"complete"`
	ObservedAt         time.Time             `json:"observed_at"`
	Resources          []ResourceObservation `json:"resources"`
}

// Resource is a persisted inventory record.
type Resource struct {
	Key              ResourceKey       `json:"key"`
	Kind             ResourceKind      `json:"kind"`
	DisplayName      string            `json:"display_name,omitempty"`
	LifecycleState   string            `json:"lifecycle_state,omitempty"`
	Attributes       map[string]string `json:"attributes,omitempty"`
	Endpoints        []Endpoint        `json:"endpoints,omitempty"`
	Presence         Presence          `json:"presence"`
	MissingSnapshots uint32            `json:"missing_snapshots"`
	FirstSeenAt      time.Time         `json:"first_seen_at"`
	LastSeenAt       time.Time         `json:"last_seen_at"`
	MissingSince     *time.Time        `json:"missing_since,omitempty"`
}

// ProviderStatus is safe operational evidence for a provider instance.
type ProviderStatus struct {
	ProviderID         ProviderID         `json:"provider"`
	ProviderInstanceID ProviderInstanceID `json:"provider_instance"`
	Enabled            bool               `json:"enabled"`
	LastAttemptAt      time.Time          `json:"last_attempt_at,omitempty"`
	LastSuccessAt      time.Time          `json:"last_success_at,omitempty"`
	LastErrorCode      string             `json:"last_error_code,omitempty"`
	ResourceCount      int                `json:"resource_count"`
}

// InventorySnapshot is a consistent material revision and its records.
type InventorySnapshot struct {
	Revision  uint64           `json:"inventory_revision"`
	Resources []Resource       `json:"resources"`
	Providers []ProviderStatus `json:"providers"`
}

// InventoryRepository persists inventory snapshots independently of desired state.
type InventoryRepository interface {
	Snapshot() (InventorySnapshot, error)
	CommitProviderSnapshot(expectedRevision uint64, snapshot ProviderSnapshot, resources []Resource, materialChange bool) error
	RecordProviderFailure(status ProviderStatus) error
}

// DiscoveryProvider returns one complete snapshot or an explicit error.
type DiscoveryProvider interface {
	ID() ProviderID
	InstanceID() ProviderInstanceID
	Discover(ctx context.Context) (ProviderSnapshot, error)
}
