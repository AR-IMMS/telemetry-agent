package main

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentlifecycle"
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/config"
	"github.com/ar-imms/telemetry-agent/internal/dependency"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

func TestManagedAgentUninstallRequesterUsesPersistedCollectorContext(
	t *testing.T,
) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	store := agentstate.NewFileStore(statePath)

	if err := store.Save(agentstate.State{
		Collector: agentstate.CollectorContext{
			ConfigRoot:      "/srv/ar-imms/config",
			BinaryPath:      "/opt/ar-imms/collector/otelcol-contrib",
			ConfigPath:      "/var/lib/ar-imms/telemetry-agent/collector.yaml",
			GatewayEndpoint: "gateway.example:4317",
		},
		Dependencies: map[string]agentstate.DependencyState{
			"node-exporter": {Enabled: true},
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	platform := identity.PlatformInfo{
		OS:           "linux",
		Architecture: "amd64",
	}
	runner := bootstrap.OSCommandRunner{}
	layers := []config.Layer{
		{Name: "base", Path: "/srv/ar-imms/config/base/otel.yaml"},
	}

	catalog, err := dependency.NewCatalog(nil)
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}

	called := false

	requester := managedAgentUninstallRequester{
		collectPlatform: func() (identity.PlatformInfo, error) {
			return platform, nil
		},
		buildCatalog: func() (dependency.Catalog, error) {
			return catalog, nil
		},
		layersFor: func(
			configRoot string,
			osName string,
		) ([]config.Layer, error) {
			if configRoot != "/srv/ar-imms/config" {
				t.Fatalf("config root = %q", configRoot)
			}
			if osName != "linux" {
				t.Fatalf("OS = %q", osName)
			}

			return layers, nil
		},
		runner: runner,
		requestAll: func(
			_ context.Context,
			gotStore agentstate.FileStore,
			options agentlifecycle.CollectorConfigurationOptions,
		) ([]string, error) {
			called = true

			if got, want := options.Platform, platform; got != want {
				t.Fatalf("platform = %#v, want %#v", got, want)
			}
			if !reflect.DeepEqual(options.Catalog, catalog) {
				t.Fatalf(
					"catalog = %#v, want %#v",
					options.Catalog,
					catalog,
				)
			}
			if !reflect.DeepEqual(options.Layers, layers) {
				t.Fatalf(
					"layers = %#v, want %#v",
					options.Layers,
					layers,
				)
			}
			if options.BinaryPath !=
				"/opt/ar-imms/collector/otelcol-contrib" {
				t.Fatalf("binary path = %q", options.BinaryPath)
			}
			if options.ConfigPath !=
				"/var/lib/ar-imms/telemetry-agent/collector.yaml" {
				t.Fatalf("config path = %q", options.ConfigPath)
			}
			if got, want := options.ValidationEnvironment,
				[]string{
					"OTEL_GATEWAY_ENDPOINT=gateway.example:4317",
				}; !reflect.DeepEqual(got, want) {
				t.Fatalf(
					"validation environment = %v, want %v",
					got,
					want,
				)
			}

			state, err := gotStore.Load()
			if err != nil {
				t.Fatalf("request store Load() error = %v", err)
			}
			if _, exists := state.Dependencies["node-exporter"]; !exists {
				t.Fatal("request store did not contain persisted dependency")
			}

			return []string{"node-exporter"}, nil
		},
	}

	if err := requester.Request(context.Background(), statePath); err != nil {
		t.Fatalf("Request() error = %v", err)
	}
	if !called {
		t.Fatal("batch dependency uninstall request was not called")
	}
}
