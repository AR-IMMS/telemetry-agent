package main

import (
	"log"
	"os"
	"strings"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/discovery"
	discoverydocker "github.com/ar-imms/telemetry-agent/internal/discovery/docker"
)

const (
	dockerDiscoveryEndpointEnv = "AR_IMMS_DOCKER_DISCOVERY_ENDPOINT"
	discoveryProviderTimeout   = 5 * time.Second
)

// newDockerDiscoveryCoordinator opts in only when an explicit local endpoint is set.
func newDockerDiscoveryCoordinator(statePath string) *discovery.Coordinator {
	endpoint := strings.TrimSpace(os.Getenv(dockerDiscoveryEndpointEnv))
	if endpoint == "" {
		return nil
	}
	repository := agentstate.NewSQLiteStore(statePath).InventoryRepository()
	provider, err := discoverydocker.NewProvider(endpoint)
	if err != nil {
		status := discovery.ProviderStatus{
			ProviderID: "docker", ProviderInstanceID: "configured", Enabled: true,
			LastAttemptAt: time.Now().UTC(), LastErrorCode: "invalid_configuration",
		}
		if recordErr := repository.RecordProviderFailure(status); recordErr != nil {
			log.Printf("Docker discovery configuration diagnostic could not persist code=inventory_store_error")
		}
		log.Printf("Docker discovery is enabled but the endpoint is invalid code=invalid_configuration")
		return nil
	}
	coordinator, err := discovery.NewCoordinator(repository, []discovery.DiscoveryProvider{provider}, discovery.DefaultPolicy(), discoveryProviderTimeout)
	if err != nil {
		log.Printf("Docker discovery could not start code=invalid_configuration")
		return nil
	}
	return coordinator
}
