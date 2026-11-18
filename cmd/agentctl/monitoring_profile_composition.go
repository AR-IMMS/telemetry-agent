package main

import (
	inventorydomain "github.com/ar-imms/telemetry-agent/internal/inventory/observation/domain"
	profilesapplication "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/application"
	monitoringdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/domain"
	yamlprofiles "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/yaml"
	"github.com/ar-imms/telemetry-agent/internal/runtime/collector/contracts"
)

// loadMonitoringProfiles composes Monitoring policy from the built-in profile
// and validated local definitions at the executable boundary.
func loadMonitoringProfiles(configRoot string) (monitoringdomain.Registry, error) {
	return profilesapplication.LoadDirectory(configRoot, yamlprofiles.DirectorySource{})
}

// monitoringEndpointEvidence projects persisted Inventory endpoints into the
// narrow evidence contract consumed by Monitoring profile diagnostics.
func monitoringEndpointEvidence(endpoints []inventorydomain.Endpoint) []profilesapplication.EndpointObservation {
	result := make([]profilesapplication.EndpointObservation, 0, len(endpoints))
	for _, endpoint := range endpoints {
		result = append(result, profilesapplication.EndpointObservation{
			Address: endpoint.Address, NetworkName: endpoint.NetworkName,
			NetworkPriority: endpoint.NetworkPriority, Port: endpoint.Port,
			ContainerPort: endpoint.ContainerPort, Protocol: endpoint.Protocol,
			Scope: monitoringdomain.EndpointScope(endpoint.Scope), Provenance: endpoint.Provenance,
		})
	}
	return result
}

func diagnoseInventoryEndpoint(profile monitoringdomain.Profile, platform string, endpoints []inventorydomain.Endpoint, evidence []profilesapplication.CollectorScrapeEvidence, attributes map[string]string) profilesapplication.EndpointDiagnostic {
	return profilesapplication.DiagnoseEndpointWithAttributes(profile, platform, monitoringEndpointEvidence(endpoints), evidence, attributes)
}

// collectorMonitoringProfiles translates Monitoring-owned intent into the
// narrow Runtime Collector input contract at the composition boundary.
func collectorMonitoringProfiles(profiles []monitoringdomain.Profile) []contracts.DockerMonitoringProfile {
	result := make([]contracts.DockerMonitoringProfile, 0, len(profiles))
	for _, profile := range profiles {
		translated := contracts.DockerMonitoringProfile{
			ID: profile.ID, Provider: profile.Provider,
			MatchLabels:          cloneMonitoringLabels(profile.MatchLabels),
			RequiredCapabilities: append([]string(nil), profile.RequiredCapabilities...),
			ReceiverType:         profile.ReceiverType, CollectionInterval: profile.CollectionInterval,
			Settings: contracts.DockerReceiverSettings{
				MetricsPath: profile.Settings.MetricsPath, UsernameEnv: profile.Settings.UsernameEnv,
				PasswordEnv: profile.Settings.PasswordEnv, TLSCAFile: profile.Settings.TLSCAFile,
			},
		}
		for _, strategy := range profile.EndpointStrategies {
			translated.EndpointStrategies = append(translated.EndpointStrategies, contracts.DockerEndpointStrategy{
				ID: strategy.ID, OS: strategy.OS, When: strategy.When, Endpoint: strategy.Endpoint,
				Scope: string(strategy.Scope), Provenance: strategy.Provenance, TargetPort: strategy.TargetPort,
				TargetPortLabel: strategy.TargetPortLabel, PrivatePortVariable: strategy.PrivatePortVariable,
				Priority: strategy.Priority,
			})
		}
		result = append(result, translated)
	}
	return result
}

func cloneMonitoringLabels(labels map[string]string) map[string]string {
	copyOfLabels := make(map[string]string, len(labels))
	for key, value := range labels {
		copyOfLabels[key] = value
	}
	return copyOfLabels
}
