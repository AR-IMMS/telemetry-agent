package application

import (
	"fmt"

	"github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/domain"
)

// ProfileSource supplies validated profile definitions from a configuration source.
type ProfileSource interface {
	Load(configRoot string) ([]domain.Profile, error)
}

// LoadDirectory combines the built-in Redis definition with local profile definitions.
func LoadDirectory(configRoot string, source ProfileSource) (domain.Registry, error) {
	if source == nil {
		return domain.Registry{}, fmt.Errorf("monitoring profile source is required")
	}
	profiles := []domain.Profile{domain.RedisProfile()}
	local, err := source.Load(configRoot)
	if err != nil {
		return domain.Registry{}, err
	}
	profiles = append(profiles, local...)
	return domain.NewRegistry(profiles...)
}
