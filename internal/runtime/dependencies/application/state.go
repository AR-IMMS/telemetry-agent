package dependencyapplication

import (
	"fmt"
	"sort"
	"strings"

	dependencydomain "github.com/ar-imms/telemetry-agent/internal/runtime/dependencies/domain"
)

// Snapshot is the dependency-owned projection of desired lifecycle state.
type Snapshot struct {
	DesiredGeneration uint64
	AppliedGeneration uint64
	Dependencies      map[string]dependencydomain.DependencyState
}

// EnableDependency enables one dependency, cancels its pending teardown, and
// advances the desired generation only when its enabled state changes.
func EnableDependency(current Snapshot, name string) (Snapshot, bool, error) {
	normalizedName := normalizeName(name)
	if normalizedName == "" {
		return current, false, fmt.Errorf("dependency name is required")
	}

	dependency, exists := current.Dependencies[normalizedName]
	if exists && dependency.Enabled {
		return current, false, nil
	}

	next := cloneSnapshot(current)
	dependency.Enabled = true
	dependency.PendingTeardown = nil
	next.Dependencies[normalizedName] = dependency
	next.DesiredGeneration++
	return next, true, nil
}

// DisableDependency disables an enabled dependency and records teardown after
// the Collector applies the new desired generation.
func DisableDependency(current Snapshot, name string) (Snapshot, bool, error) {
	normalizedName := normalizeName(name)
	if normalizedName == "" {
		return current, false, fmt.Errorf("dependency name is required")
	}

	dependency, exists := current.Dependencies[normalizedName]
	if !exists || !dependency.Enabled {
		return current, false, nil
	}

	next := cloneSnapshot(current)
	dependency.Enabled = false
	next.DesiredGeneration++
	dependency.PendingTeardown = &dependencydomain.PendingTeardown{
		Action:     dependencydomain.TeardownActionDisable,
		Generation: next.DesiredGeneration,
	}
	next.Dependencies[normalizedName] = dependency
	return next, true, nil
}

// RequestUninstall removes one dependency from desired configuration and waits
// for the Collector to apply the generation before host-resource deletion.
func RequestUninstall(current Snapshot, name string) (Snapshot, bool, error) {
	normalizedName := normalizeName(name)
	if normalizedName == "" {
		return current, false, fmt.Errorf("dependency name is required")
	}

	dependency, exists := current.Dependencies[normalizedName]
	if !exists {
		return current, false, fmt.Errorf("dependency %q has no persisted Agent state", normalizedName)
	}
	if dependency.PendingTeardown != nil && dependency.PendingTeardown.Action == dependencydomain.TeardownActionUninstall {
		return current, false, nil
	}

	next := cloneSnapshot(current)
	dependency.Enabled = false
	next.DesiredGeneration++
	dependency.PendingTeardown = &dependencydomain.PendingTeardown{
		Action:     dependencydomain.TeardownActionUninstall,
		Generation: next.DesiredGeneration,
	}
	next.Dependencies[normalizedName] = dependency
	return next, true, nil
}

// CompleteTeardown clears a disable intent or removes an uninstalled
// dependency after the requested generation has been applied.
func CompleteTeardown(current Snapshot, name string, generation uint64) (Snapshot, error) {
	normalizedName := normalizeName(name)
	if normalizedName == "" {
		return current, fmt.Errorf("dependency name is required")
	}

	dependency, exists := current.Dependencies[normalizedName]
	if !exists {
		return current, fmt.Errorf("dependency %q has no persisted Agent state", normalizedName)
	}
	if dependency.PendingTeardown == nil {
		return current, fmt.Errorf("dependency %q has no pending teardown", normalizedName)
	}
	if dependency.PendingTeardown.Generation != generation {
		return current, fmt.Errorf(
			"dependency %q pending teardown generation is %d, not %d",
			normalizedName,
			dependency.PendingTeardown.Generation,
			generation,
		)
	}
	if current.AppliedGeneration < generation {
		return current, fmt.Errorf("dependency %q teardown generation %d is not applied", normalizedName, generation)
	}

	next := cloneSnapshot(current)
	switch dependency.PendingTeardown.Action {
	case dependencydomain.TeardownActionDisable:
		dependency.PendingTeardown = nil
		next.Dependencies[normalizedName] = dependency
	case dependencydomain.TeardownActionUninstall:
		delete(next.Dependencies, normalizedName)
	default:
		return current, fmt.Errorf("dependency %q has unsupported teardown action %q", normalizedName, dependency.PendingTeardown.Action)
	}
	return next, nil
}

// RequestUninstalls schedules multiple teardown intents for one shared
// generation after validating every requested dependency.
func RequestUninstalls(current Snapshot, names []string) (Snapshot, []string, error) {
	if len(names) == 0 {
		return current, nil, fmt.Errorf("at least one dependency is required")
	}

	normalizedNames := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		normalizedName := normalizeName(name)
		if normalizedName == "" {
			return current, nil, fmt.Errorf("dependency name is required")
		}
		if _, exists := seen[normalizedName]; exists {
			continue
		}
		seen[normalizedName] = struct{}{}
		normalizedNames = append(normalizedNames, normalizedName)
	}
	sort.Strings(normalizedNames)

	for _, name := range normalizedNames {
		if _, exists := current.Dependencies[name]; !exists {
			return current, nil, fmt.Errorf("dependency %q has no persisted Agent state", name)
		}
	}

	changedNames := make([]string, 0, len(normalizedNames))
	for _, name := range normalizedNames {
		dependency := current.Dependencies[name]
		if dependency.PendingTeardown == nil || dependency.PendingTeardown.Action != dependencydomain.TeardownActionUninstall {
			changedNames = append(changedNames, name)
		}
	}
	if len(changedNames) == 0 {
		return current, nil, nil
	}

	next := cloneSnapshot(current)
	next.DesiredGeneration++
	for _, name := range changedNames {
		dependency := next.Dependencies[name]
		dependency.Enabled = false
		dependency.PendingTeardown = &dependencydomain.PendingTeardown{
			Action:     dependencydomain.TeardownActionUninstall,
			Generation: next.DesiredGeneration,
		}
		next.Dependencies[name] = dependency
	}
	return next, changedNames, nil
}

func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func cloneSnapshot(current Snapshot) Snapshot {
	next := current
	next.Dependencies = make(map[string]dependencydomain.DependencyState, len(current.Dependencies)+1)
	for name, dependency := range current.Dependencies {
		next.Dependencies[name] = dependency
	}
	return next
}
