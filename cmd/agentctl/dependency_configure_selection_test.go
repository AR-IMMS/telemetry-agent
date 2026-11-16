package main

import (
	"reflect"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/runtime/dependencies"
)

func TestDependencyConfigureSelectionPreservesInitialDesiredStates(t *testing.T) {
	selection := newDependencyConfigureSelection([]dependencyConfigureOption{
		{Definition: dependency.Definition{Name: "windows-exporter"}, Enabled: true},
		{Definition: dependency.Definition{Name: "libre-hardware-monitor"}, Enabled: false},
	})
	want := map[string]bool{"windows-exporter": true, "libre-hardware-monitor": false}
	if got := selection.DesiredStates(); !reflect.DeepEqual(got, want) {
		t.Fatalf("DesiredStates() = %#v, want %#v", got, want)
	}
	got := selection.DesiredStates()
	got["windows-exporter"] = false
	if selection.desiredStates["windows-exporter"] != true {
		t.Fatal("DesiredStates() exposed mutable selection state")
	}
}
