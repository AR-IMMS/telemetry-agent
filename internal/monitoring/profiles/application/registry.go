package application

import (
	"fmt"

	monitoringprofile "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles"
)

// ProfileSource supplies validated profile definitions from a configuration source.
type ProfileSource interface {
	Load(configRoot string) ([]monitoringprofile.Profile, error)
}

// LoadDirectory combines the built-in Redis definition with local profile definitions.
func LoadDirectory(configRoot string, source ProfileSource) (monitoringprofile.Registry, error) {
	if source == nil {
		return monitoringprofile.Registry{}, fmt.Errorf("monitoring profile source is required")
	}
	profiles := []monitoringprofile.Profile{monitoringprofile.RedisProfile()}
	local, err := source.Load(configRoot)
	if err != nil {
		return monitoringprofile.Registry{}, err
	}
	profiles = append(profiles, local...)
	return monitoringprofile.NewRegistry(profiles...)
}
