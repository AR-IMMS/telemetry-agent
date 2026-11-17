package main

import (
	"errors"
	"fmt"
	desiredstatedomain "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
	enrollment "github.com/ar-imms/telemetry-agent/internal/identity/enrollment/application"
	"github.com/ar-imms/telemetry-agent/internal/identity/host"
	inventoryctx "github.com/ar-imms/telemetry-agent/internal/inventory"
	definitionsdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/domain"
	monitoringdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/domain"
	"github.com/ar-imms/telemetry-agent/internal/runtime/collector"
	config "github.com/ar-imms/telemetry-agent/internal/runtime/collector/configuration"
	"os"
	"runtime"
	"strings"
)

// nativeDockerMonitoringInputs opts into OTel-native profile discovery only
// when an explicit local Docker endpoint and a registered Core node are known.
func nativeDockerMonitoringInputs(statePath, osName string) ([]config.InlineLayer, []string, error) {
	if strings.TrimSpace(os.Getenv(dockerDiscoveryEndpointEnv)) == "" {
		return nil, nil, nil
	}
	state, err := newAgentStateStore(statePath).Load()
	if err != nil {
		return nil, nil, fmt.Errorf("load Collector config root for native Docker monitoring: %w", err)
	}
	profiles, err := loadMonitoringProfiles(state.Collector.ConfigRoot)
	if err != nil {
		return nil, nil, err
	}
	return nativeDockerMonitoringInputsWithProfiles(statePath, osName, profiles)
}

func nativeDockerMonitoringInputsWithConfigRoot(statePath, osName, configRoot string) ([]config.InlineLayer, []string, error) {
	if strings.TrimSpace(os.Getenv(dockerDiscoveryEndpointEnv)) == "" {
		return nil, nil, nil
	}
	profiles, err := loadMonitoringProfiles(configRoot)
	if err != nil {
		return nil, nil, err
	}
	return nativeDockerMonitoringInputsWithProfiles(statePath, osName, profiles)
}

func nativeDockerMonitoringInputsWithProfiles(statePath, osName string, profiles monitoringdomain.Registry) ([]config.InlineLayer, []string, error) {
	endpoint := strings.TrimSpace(os.Getenv(dockerDiscoveryEndpointEnv))
	if endpoint == "" {
		return nil, nil, nil
	}
	attempt, err := newAgentEnrollmentStore(statePath).LoadAttempt()
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
	layer, receivers, err := collectorruntime.DockerMonitoringLayer(collectorMonitoringProfiles(profiles.Profiles()), strings.ToLower(osName), endpoint, nodeID)
	if err != nil {
		return nil, nil, err
	}
	return []config.InlineLayer{layer}, receivers, nil
}

type nativeOwnershipResolver struct {
	delegate  contributionResolver
	inventory interface {
		Snapshot() (inventoryctx.InventorySnapshot, error)
	}
	profiles monitoringdomain.Registry
	enabled  bool
}

type contributionResolver interface {
	ResolveWithContribution(desiredstatedomain.Instance) (desiredstatedomain.Instance, definitionsdomain.Contribution, error)
}

func (r nativeOwnershipResolver) Resolve(instance desiredstatedomain.Instance) (definitionsdomain.Contribution, error) {
	if !r.enabled || instance.Binding == nil {
		_, contribution, err := r.delegate.ResolveWithContribution(instance)
		return contribution, err
	}
	snapshot, err := r.inventory.Snapshot()
	if err != nil {
		return definitionsdomain.Contribution{}, fmt.Errorf("read inventory for native receiver ownership: %w", err)
	}
	for _, resource := range snapshot.Resources {
		binding := instance.Binding
		if string(resource.Key.Provider) != binding.Provider || string(resource.Key.ProviderInstance) != binding.ProviderInstance || resource.Key.ProviderResourceID != binding.ResourceID {
			continue
		}
		if profile, matched := r.profiles.Match(string(resource.Key.Provider), resource.Attributes); matched && profile.ID == instance.DefinitionID {
			_, _, err := r.delegate.ResolveWithContribution(instance)
			if err != nil {
				return definitionsdomain.Contribution{}, err
			}
			return definitionsdomain.Contribution{}, nil
		}
	}
	_, contribution, err := r.delegate.ResolveWithContribution(instance)
	return contribution, err
}

var _ interface {
	Resolve(desiredstatedomain.Instance) (definitionsdomain.Contribution, error)
} = nativeOwnershipResolver{}
