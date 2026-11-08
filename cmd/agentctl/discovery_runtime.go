package main

import (
	"context"
	"errors"
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
	discoveryPollInterval      = 30 * time.Second
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

// runRuntimeControlLoops runs discovery beside the Agent Service's sole S3 controller.
// Discovery errors are logged as safe codes and never stop Collector supervision.
func runRuntimeControlLoops(ctx context.Context, reconcile func(context.Context) error, inventory *discovery.Coordinator) error {
	if inventory == nil {
		return reconcile(ctx)
	}
	serviceContext, cancel := context.WithCancel(ctx)
	discoveryDone := make(chan struct{})
	go func() {
		defer close(discoveryDone)
		err := inventory.Run(serviceContext, discoveryPollInterval, func(error) {
			log.Printf("Discovery inventory cycle failed code=inventory_store_error")
		})
		if err != nil && !errors.Is(err, context.Canceled) && serviceContext.Err() == nil {
			log.Printf("Discovery service stopped code=inventory_service_error")
		}
	}()
	err := reconcile(serviceContext)
	cancel()
	<-discoveryDone
	return err
}
