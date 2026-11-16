package main

import (
	"github.com/ar-imms/telemetry-agent/internal/monitoring/profiles"
	profilesapplication "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/application"
	yamlprofiles "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/yaml"
	"github.com/ar-imms/telemetry-agent/internal/runtime/collector/contracts"
)

// loadMonitoringProfiles composes Monitoring policy from the built-in profile
// and validated local definitions at the executable boundary.
func loadMonitoringProfiles(configRoot string) (monitoringprofile.Registry, error) {
	return profilesapplication.LoadDirectory(configRoot, yamlprofiles.DirectorySource{})
}

// collectorMonitoringProfiles translates Monitoring-owned intent into the
// narrow Runtime Collector input contract at the composition boundary.
func collectorMonitoringProfiles(profiles []monitoringprofile.Profile) []contracts.DockerMonitoringProfile {
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
