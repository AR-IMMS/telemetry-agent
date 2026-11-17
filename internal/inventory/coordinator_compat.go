package inventory

import (
	"time"

	"github.com/ar-imms/telemetry-agent/internal/inventory/observation/application"
)

// CycleResult contains per-provider outcomes from one discovery cycle.
type CycleResult = application.CycleResult

// ProviderResult contains one provider instance's safe outcome.
type ProviderResult = application.ProviderResult

// Coordinator serializes discovery and commits normalized inventory snapshots.
type Coordinator = application.Coordinator

// NewCoordinator creates the compatibility entry point for inventory discovery.
func NewCoordinator(repository InventoryRepository, providers []DiscoveryProvider, policy Policy, timeout time.Duration) (*Coordinator, error) {
	return application.NewCoordinator(repository, providers, policy, timeout)
}
