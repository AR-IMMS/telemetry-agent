package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agenthealth"
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/credentialstore"
	"github.com/ar-imms/telemetry-agent/internal/dependency"
	"github.com/ar-imms/telemetry-agent/internal/discovery/docker"
	"github.com/ar-imms/telemetry-agent/internal/enrollment"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	identitysqlite "github.com/ar-imms/telemetry-agent/internal/identity/sqlite"
	"github.com/ar-imms/telemetry-agent/internal/operations"
	"github.com/ar-imms/telemetry-agent/internal/operations/handlers"
	operationsnats "github.com/ar-imms/telemetry-agent/internal/operations/nats"
	operationssqlite "github.com/ar-imms/telemetry-agent/internal/operations/sqlite"
)

const (
	operationsURLEnv         = "AR_IMMS_OPERATIONS_NATS_URL"
	operationsUserEnv        = "AR_IMMS_OPERATIONS_NATS_USER"
	operationsPasswordRefEnv = "AR_IMMS_OPERATIONS_NATS_PASSWORD_REF"
	operationsCAEnv          = "AR_IMMS_OPERATIONS_NATS_CA_FILE"
	operationsBindingIDEnv   = "AR_IMMS_OPERATIONS_BINDING_ID"
	operationsRevisionEnv    = "AR_IMMS_OPERATIONS_BINDING_REVISION"
	operationsStreamEnv      = "AR_IMMS_OPERATIONS_STREAM"
	operationsDurableEnv     = "AR_IMMS_OPERATIONS_DURABLE"
)

type dockerOperationsInspector struct{ provider *docker.Provider }

func (a dockerOperationsInspector) Inspect(ctx context.Context, id string) (handlers.ContainerObservation, error) {
	if a.provider == nil {
		return handlers.ContainerObservation{}, fmt.Errorf("Docker provider is unavailable")
	}
	observation, err := a.provider.InspectContainer(ctx, id)
	if err != nil {
		return handlers.ContainerObservation{}, err
	}
	return handlers.ContainerObservation{ID: observation.ID, Name: observation.Name, Image: observation.Image,
		State: observation.State, Health: observation.Health, RestartCount: observation.RestartCount, StartedAt: observation.StartedAt}, nil
}

type dependencyOperationsInspector struct{ service dependency.Service }

func (a dependencyOperationsInspector) Inspect(ctx context.Context, name string) (handlers.DependencyObservation, error) {
	status, err := a.service.Status(ctx, name)
	if err != nil {
		return handlers.DependencyObservation{}, err
	}
	return handlers.DependencyObservation{Name: name, Availability: string(status.Availability), Health: string(status.Health), Drifted: status.Drifted}, nil
}

type managedOperationsRunner struct {
	runner   *operationsnats.Runner
	journal  *operationssqlite.Repository
	lockPath string
}

func (r *managedOperationsRunner) Run(ctx context.Context) error {
	if ctx == nil || r == nil || r.runner == nil || r.journal == nil || r.lockPath == "" {
		return fmt.Errorf("Operations runtime runner, journal, lock path, and context are required")
	}
	var lock agentstate.LifecycleLock
	for ctx.Err() == nil {
		var err error
		lock, err = agentstate.AcquireLifecycleLock(r.lockPath)
		if err == nil {
			break
		}
		log.Printf("Remote Operations waiting for the installation execution lock")
		if !waitOperationsRetry(ctx) {
			return nil
		}
	}
	if ctx.Err() != nil {
		return nil
	}
	defer lock.Close()
	for ctx.Err() == nil {
		if _, err := r.journal.RecoverInterrupted(ctx, time.Now().UTC()); err == nil {
			break
		} else {
			log.Printf("Remote Operations startup recovery failed code=operations_recovery_failed")
		}
		if !waitOperationsRetry(ctx) {
			return nil
		}
	}
	if ctx.Err() != nil {
		return nil
	}
	return r.runner.Run(ctx)
}

func waitOperationsRetry(ctx context.Context) bool {
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func newRuntimeOperationsRunner(statePath string, reporter agenthealth.ObservationReporter) *managedOperationsRunner {
	url := strings.TrimSpace(os.Getenv(operationsURLEnv))
	if url == "" {
		return nil
	}
	store := agentstate.NewSQLiteStore(statePath)
	db, err := store.OpenDatabase()
	if err != nil {
		log.Printf("Remote Operations disabled code=operations_identity_unavailable")
		return nil
	}
	installationID, initialized, err := identitysqlite.ReadInstallationID(db)
	closeErr := db.Close()
	if err != nil || closeErr != nil || !initialized {
		log.Printf("Remote Operations disabled code=operations_identity_unavailable")
		return nil
	}
	bindingID := strings.TrimSpace(os.Getenv(operationsBindingIDEnv))
	passwordRef := strings.TrimSpace(os.Getenv(operationsPasswordRefEnv))
	config := operationsnats.Config{URL: url, Username: os.Getenv(operationsUserEnv),
		CAFile: strings.TrimSpace(os.Getenv(operationsCAEnv)), InstallationID: installationID.String(), BindingID: bindingID,
		Stream: strings.TrimSpace(os.Getenv(operationsStreamEnv)), Durable: strings.TrimSpace(os.Getenv(operationsDurableEnv))}
	if config.Username == "" || config.CAFile == "" || config.BindingID == "" || config.Stream == "" || config.Durable == "" {
		log.Printf("Remote Operations disabled code=operations_configuration_incomplete")
		return nil
	}
	revision := strings.TrimSpace(os.Getenv(operationsRevisionEnv))
	if revision == "" {
		revision = "protected-credential-current"
	}
	credentials := credentialstore.New(statePath)
	loadBinding := operationsBindingLoader(config, revision, passwordRef, credentials.Get)
	var healthReader handlers.HealthReader
	if provider, ok := reporter.(agenthealth.ContextProvider); ok {
		healthReader = provider
	}
	registry := operations.NewRegistry()
	dockerEndpoint := strings.TrimSpace(os.Getenv(dockerDiscoveryEndpointEnv))
	if dockerEndpoint == "" {
		if runtime.GOOS == "windows" {
			dockerEndpoint = "npipe:////./pipe/docker_engine"
		} else {
			dockerEndpoint = "unix:///var/run/docker.sock"
		}
	}
	var containerInspector handlers.ContainerInspector
	if provider, err := docker.NewProvider(dockerEndpoint); err == nil {
		containerInspector = dockerOperationsInspector{provider: provider}
	} else {
		log.Printf("Remote container inspection unavailable code=operations_docker_adapter_error")
	}
	var dependencyInspector handlers.DependencyInspector
	platform, platformErr := identity.CollectPlatformInfo()
	if catalog, catalogErr := defaultDependencyCatalog(); catalogErr == nil && platformErr == nil {
		dependencyInspector = dependencyOperationsInspector{service: dependency.Service{Catalog: catalog, OS: platform.OS}}
	} else {
		log.Printf("Remote dependency inspection unavailable code=operations_dependency_catalog_error")
	}
	for _, registration := range []func() error{
		func() error { return operations.Register(registry, handlers.Diagnostics{Health: healthReader}) },
		func() error {
			return operations.Register(registry, handlers.ContainerInspect{Containers: containerInspector})
		},
		func() error {
			return operations.Register(registry, handlers.DependencyInspect{Dependencies: dependencyInspector})
		},
	} {
		if err := registration(); err != nil {
			log.Printf("Remote Operations disabled code=operations_registry_error")
			return nil
		}
	}
	journal := operationssqlite.New(store)
	executor := &operations.Executor{Registry: registry, Store: journal, InstallationID: installationID.String(), BindingID: bindingID}
	return &managedOperationsRunner{runner: &operationsnats.Runner{Config: config, Binding: loadBinding, Executor: executor, Outbox: journal},
		journal: journal, lockPath: statePath + ".operations"}
}

func operationsBindingLoader(config operationsnats.Config, revision, credentialRef string,
	getCredential func(string) (string, error)) operationsnats.BindingLoader {
	return func(context.Context) (operationsnats.BindingState, error) {
		password, err := getCredential(credentialRef)
		if err != nil {
			if errors.Is(err, enrollment.ErrCredentialNotFound) {
				return operationsnats.BindingState{SchemaVersion: operationsnats.BindingSchemaV1,
					Revision: revision, Active: false}, nil
			}
			return operationsnats.BindingState{}, err
		}
		current := config
		current.Password = password
		return operationsnats.BindingState{SchemaVersion: operationsnats.BindingSchemaV1,
			Revision: revision, Active: true, Config: current}, nil
	}
}

var _ interface {
	Inspect(context.Context, string) (handlers.DependencyObservation, error)
} = dependencyOperationsInspector{}
