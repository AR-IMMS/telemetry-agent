package agentstate

import (
	"reflect"
	"testing"
)

func TestFileStorePersistsDesiredState(t *testing.T) {
	statePath := t.TempDir() + "/state.json"

	want := State{
		Collector: CollectorContext{
			ConfigRoot:      "/opt/ar-imms/configs",
			BinaryPath:      "/opt/ar-imms/otelcol-contrib",
			ConfigPath:      "/etc/ar-imms/otel.yaml",
			GatewayEndpoint: "127.0.0.1:4317",
			HealthEndpoint:  "http://127.0.0.1:13133",
		},
		DesiredGeneration: 3,
		Dependencies: map[string]DependencyState{
			"node-exporter": {
				Enabled: true,
			},
		},
	}

	store := NewFileStore(statePath)

	if err := store.Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
}

func TestFileStoreUpdateCreatesMissingState(t *testing.T) {
	store := NewFileStore(t.TempDir() + "/state.json")

	got, err := store.Update(func(state *State) error {
		state.Collector.ConfigRoot = "/etc/ar-imms/configs"

		return nil
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if got.Collector.ConfigRoot != "/etc/ar-imms/configs" {
		t.Fatalf(
			"updated ConfigRoot = %q, want /etc/ar-imms/configs",
			got.Collector.ConfigRoot,
		)
	}

	persisted, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(persisted, got) {
		t.Fatalf("persisted state = %#v, want %#v", persisted, got)
	}
}
