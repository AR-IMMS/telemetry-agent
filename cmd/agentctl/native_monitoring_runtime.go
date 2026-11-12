package main

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/collectorruntime"
	"github.com/ar-imms/telemetry-agent/internal/config"
	"github.com/ar-imms/telemetry-agent/internal/discovery"
	"github.com/ar-imms/telemetry-agent/internal/enrollment"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	"github.com/ar-imms/telemetry-agent/internal/integration"
	"github.com/ar-imms/telemetry-agent/internal/monitoringprofile"
)

// nativeDockerMonitoringInputs opts into OTel-native profile discovery only
// when an explicit local Docker endpoint and a registered Core node are known.
func nativeDockerMonitoringInputs(statePath, osName string) ([]config.InlineLayer, []string, error) {
	if strings.TrimSpace(os.Getenv(dockerDiscoveryEndpointEnv)) == "" {
		return nil, nil, nil
	}
	state, err := agentstate.NewStore(statePath).Load()
	if err != nil {
		return nil, nil, fmt.Errorf("load Collector config root for native Docker monitoring: %w", err)
	}
	profiles, err := monitoringprofile.LoadDirectory(state.Collector.ConfigRoot)
	if err != nil {
		return nil, nil, err
	}
	return nativeDockerMonitoringInputsWithProfiles(statePath, osName, profiles)
}

func nativeDockerMonitoringInputsWithConfigRoot(statePath, osName, configRoot string) ([]config.InlineLayer, []string, error) {
	if strings.TrimSpace(os.Getenv(dockerDiscoveryEndpointEnv)) == "" {
		return nil, nil, nil
	}
	profiles, err := monitoringprofile.LoadDirectory(configRoot)
	if err != nil {
		return nil, nil, err
	}
	return nativeDockerMonitoringInputsWithProfiles(statePath, osName, profiles)
}

func nativeDockerMonitoringInputsWithProfiles(statePath, osName string, profiles monitoringprofile.Registry) ([]config.InlineLayer, []string, error) {
	endpoint := strings.TrimSpace(os.Getenv(dockerDiscoveryEndpointEnv))
	if endpoint == "" {
		return nil, nil, nil
	}
	attempt, err := agentstate.NewEnrollmentStore(statePath).LoadAttempt()
	if errors.Is(err, enrollment.ErrNoAttempt) || err == nil && (attempt.Status != enrollment.AttemptRegistered || attempt.Result == nil) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("load Core enrollment for native Docker monitoring: %w", err)
	}
	nodeID := strings.TrimSpace(attempt.Result.Binding.NodeID)
	if nodeID == "" {
		return nil, nil, fmt.Errorf("registered Core Node ID is missing")
	}
	catalog, err := collectorruntime.StaticCatalog(identity.PlatformInfo{OS: strings.ToLower(osName), Architecture: runtime.GOARCH})
	if err != nil {
		return nil, nil, fmt.Errorf("resolve pinned Collector capabilities: %w", err)
	}
	available := make(map[string]struct{})
	for _, capability := range catalog.Manifest() {
		available[capability.ID] = struct{}{}
	}
	for _, profile := range profiles.Profiles() {
		for _, required := range profile.RequiredCapabilities {
			if _, ok := available[required]; !ok {
				return nil, nil, fmt.Errorf("monitoring profile %q requires unavailable Collector capability %q", profile.ID, required)
			}
		}
	}
	layer, receivers, err := profiles.DockerCollectorLayer(strings.ToLower(osName), endpoint, nodeID)
	if err != nil {
		return nil, nil, err
	}
	return []config.InlineLayer{layer}, receivers, nil
}

type nativeOwnershipResolver struct {
	delegate  resolver
	inventory interface {
		Snapshot() (discovery.InventorySnapshot, error)
	}
	profiles monitoringprofile.Registry
	enabled  bool
}

type resolver interface {
	Resolve(integration.IntegrationInstance) (integration.ResolvedInstance, error)
}

func (r nativeOwnershipResolver) Resolve(instance integration.IntegrationInstance) (integration.ResolvedInstance, error) {
	if !r.enabled || instance.Binding == nil {
		return r.delegate.Resolve(instance)
	}
	snapshot, err := r.inventory.Snapshot()
	if err != nil {
		return integration.ResolvedInstance{}, fmt.Errorf("read inventory for native receiver ownership: %w", err)
	}
	for _, resource := range snapshot.Resources {
		binding := instance.Binding
		if string(resource.Key.Provider) != binding.Provider || string(resource.Key.ProviderInstance) != binding.ProviderInstance || resource.Key.ProviderResourceID != binding.ResourceID {
			continue
		}
		if profile, matched := r.profiles.Match(string(resource.Key.Provider), resource.Attributes); matched && profile.ID == instance.DefinitionID {
			resolved, err := r.delegate.Resolve(instance)
			if err != nil {
				return integration.ResolvedInstance{}, err
			}
			return integration.ResolvedInstance{Instance: resolved.Instance, Contribution: integration.NewContribution(nil)}, nil
		}
	}
	return r.delegate.Resolve(instance)
}

var _ interface {
	Resolve(integration.IntegrationInstance) (integration.ResolvedInstance, error)
} = nativeOwnershipResolver{}
