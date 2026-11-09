package discovery

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// CycleResult contains safe per-provider outcomes.
type CycleResult struct {
	InventoryRevision uint64
	ProviderResults   []ProviderResult
}

// ProviderResult contains an identity and safe outcome code.
type ProviderResult struct {
	ProviderID         ProviderID
	ProviderInstanceID ProviderInstanceID
	ErrorCode          string
}

// Coordinator serializes provider snapshots and commits them to inventory.
type Coordinator struct {
	repository InventoryRepository
	providers  []DiscoveryProvider
	policy     Policy
	timeout    time.Duration
	now        func() time.Time
	runMu      sync.Mutex
}

// NewCoordinator validates providers and prevents duplicate provider instances.
func NewCoordinator(repository InventoryRepository, providers []DiscoveryProvider, policy Policy, timeout time.Duration) (*Coordinator, error) {
	if repository == nil {
		return nil, fmt.Errorf("inventory repository is required")
	}
	if policy.MissingAfterSnapshots == 0 || policy.StaleAfter <= 0 {
		return nil, fmt.Errorf("valid inventory policy is required")
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("provider timeout must be positive")
	}
	seen := make(map[string]struct{}, len(providers))
	copyProviders := append([]DiscoveryProvider(nil), providers...)
	for _, provider := range copyProviders {
		if provider == nil || !safeIdentifier.MatchString(string(provider.ID())) {
			return nil, fmt.Errorf("invalid discovery provider")
		}
		key := string(provider.ID()) + "/" + string(provider.InstanceID())
		if strings.TrimSpace(string(provider.InstanceID())) == "" {
			return nil, fmt.Errorf("provider instance ID is required")
		}
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("duplicate discovery provider instance")
		}
		seen[key] = struct{}{}
	}
	return &Coordinator{repository: repository, providers: copyProviders, policy: policy, timeout: timeout, now: time.Now}, nil
}

// DiscoverOnce runs one serialized cycle. Provider failures update diagnostics;
// repository errors are returned for the lifecycle host to log safely.
func (c *Coordinator) DiscoverOnce(ctx context.Context) (CycleResult, error) {
	if ctx == nil {
		return CycleResult{}, fmt.Errorf("discovery context is required")
	}
	c.runMu.Lock()
	defer c.runMu.Unlock()
	result := CycleResult{ProviderResults: make([]ProviderResult, 0, len(c.providers))}
	for _, provider := range c.providers {
		attempted := c.now().UTC()
		providerCtx, cancel := context.WithTimeout(ctx, c.timeout)
		raw, err := provider.Discover(providerCtx)
		cancel()
		providerResult := ProviderResult{ProviderID: provider.ID(), ProviderInstanceID: provider.InstanceID()}
		if err != nil {
			if errors.Is(err, context.Canceled) && ctx.Err() != nil {
				return result, ctx.Err()
			}
			providerResult.ErrorCode = safeProviderErrorCode(err)
			status := ProviderStatus{ProviderID: provider.ID(), ProviderInstanceID: provider.InstanceID(), Enabled: true, LastAttemptAt: attempted, LastErrorCode: providerResult.ErrorCode}
			if recordErr := c.repository.RecordProviderFailure(status); recordErr != nil {
				return result, fmt.Errorf("record discovery provider failure: %w", recordErr)
			}
			result.ProviderResults = append(result.ProviderResults, providerResult)
			continue
		}
		if raw.ProviderID != provider.ID() || raw.ProviderInstanceID != provider.InstanceID() {
			providerResult.ErrorCode = "identity_mismatch"
			status := ProviderStatus{ProviderID: provider.ID(), ProviderInstanceID: provider.InstanceID(), Enabled: true, LastAttemptAt: attempted, LastErrorCode: providerResult.ErrorCode}
			if recordErr := c.repository.RecordProviderFailure(status); recordErr != nil {
				return result, fmt.Errorf("record invalid discovery provider result: %w", recordErr)
			}
			result.ProviderResults = append(result.ProviderResults, providerResult)
			continue
		}
		raw.ObservedAt = attempted
		snapshot, err := NormalizeSnapshot(raw)
		if err != nil {
			providerResult.ErrorCode = "invalid_snapshot"
			status := ProviderStatus{ProviderID: provider.ID(), ProviderInstanceID: provider.InstanceID(), Enabled: true, LastAttemptAt: attempted, LastErrorCode: providerResult.ErrorCode}
			if recordErr := c.repository.RecordProviderFailure(status); recordErr != nil {
				return result, fmt.Errorf("record invalid discovery snapshot: %w", recordErr)
			}
			result.ProviderResults = append(result.ProviderResults, providerResult)
			continue
		}
		if err := c.applySnapshot(snapshot, attempted); err != nil {
			return result, err
		}
		result.ProviderResults = append(result.ProviderResults, providerResult)
	}
	inventory, err := c.repository.Snapshot()
	if err != nil {
		return result, fmt.Errorf("read inventory after discovery: %w", err)
	}
	result.InventoryRevision = inventory.Revision
	return result, nil
}

func (c *Coordinator) applySnapshot(snapshot ProviderSnapshot, at time.Time) error {
	for attempt := 0; attempt < 3; attempt++ {
		current, err := c.repository.Snapshot()
		if err != nil {
			return fmt.Errorf("read inventory before provider snapshot: %w", err)
		}
		resources, changed, err := ReconcileProviderSnapshot(current.Resources, snapshot, c.policy)
		if err != nil {
			return err
		}
		err = c.repository.CommitProviderSnapshot(current.WriteRevision, snapshot, resources, changed)
		if !errors.Is(err, ErrSnapshotConflict) {
			if err != nil {
				return fmt.Errorf("commit provider snapshot: %w", err)
			}
			return nil
		}
	}
	return ErrSnapshotConflict
}

// Run performs an initial cycle and then polls until context cancellation.
func (c *Coordinator) Run(ctx context.Context, interval time.Duration, onError func(error)) error {
	if ctx == nil || interval <= 0 {
		return fmt.Errorf("discovery context and positive interval are required")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := c.DiscoverOnce(ctx); err != nil && onError != nil && ctx.Err() == nil {
			onError(err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func safeProviderErrorCode(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return "daemon_unavailable"
	}
	if strings.Contains(strings.ToLower(err.Error()), "permission") || strings.Contains(strings.ToLower(err.Error()), "access denied") {
		return "permission_denied"
	}
	return "provider_error"
}
