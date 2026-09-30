package agentstate

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"
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

func TestLoadWaitsForUpdateToReleaseStateLock(t *testing.T) {
	store := NewFileStore(filepath.Join(t.TempDir(), "state.json"))

	if err := store.Save(State{
		DesiredGeneration:   1,
		ActivatedGeneration: 1,
		Dependencies:        map[string]DependencyState{},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	mutationStarted := make(chan struct{})
	releaseMutation := make(chan struct{})

	defer func() {
		select {
		case <-releaseMutation:
		default:
			close(releaseMutation)
		}
	}()

	updateDone := make(chan error, 1)

	go func() {
		_, err := store.Update(func(state *State) error {
			close(mutationStarted)
			<-releaseMutation

			state.DesiredGeneration = 2

			return nil
		})
		updateDone <- err
	}()

	select {
	case <-mutationStarted:
	case <-time.After(time.Second):
		t.Fatal("Update() did not begin mutation")
	}

	loadDone := make(chan error, 1)

	go func() {
		_, err := store.Load()
		loadDone <- err
	}()

	select {
	case err := <-loadDone:
		t.Fatalf(
			"Load() returned before Update() released its state lock: %v",
			err,
		)
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseMutation)

	select {
	case err := <-updateDone:
		if err != nil {
			t.Fatalf("Update() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Update() did not finish after mutation release")
	}

	select {
	case err := <-loadDone:
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Load() did not finish after Update() released its state lock")
	}
}
