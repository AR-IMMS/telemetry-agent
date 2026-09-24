package main

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

func TestDependencyMultiSelectScrollsViewportPastTenthOption(
	t *testing.T,
) {
	definitions := make([]dependency.Definition, 11)
	for index := range definitions {
		definitions[index] = dependency.Definition{
			Name: fmt.Sprintf("dependency-%02d", index+1),
		}
	}

	model := newDependencyMultiSelect(definitions)

	for range 10 {
		model.MoveDown()
	}

	if got, want := model.Current().Name, "dependency-11"; got != want {
		t.Fatalf("current dependency = %q, want %q", got, want)
	}

	visible := model.Visible()

	gotNames := make([]string, len(visible))
	for index, definition := range visible {
		gotNames[index] = definition.Name
	}

	wantNames := []string{
		"dependency-02",
		"dependency-03",
		"dependency-04",
		"dependency-05",
		"dependency-06",
		"dependency-07",
		"dependency-08",
		"dependency-09",
		"dependency-10",
		"dependency-11",
	}

	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf(
			"visible dependencies = %v, want %v",
			gotNames,
			wantNames,
		)
	}
}

func TestDependencyMultiSelectPreservesSelectionAcrossScrolling(
	t *testing.T,
) {
	definitions := make([]dependency.Definition, 11)
	for index := range definitions {
		definitions[index] = dependency.Definition{
			Name: fmt.Sprintf("dependency-%02d", index+1),
		}
	}

	model := newDependencyMultiSelect(definitions)

	for range 10 {
		model.MoveDown()
	}

	model.ToggleCurrent()

	for range 10 {
		model.MoveUp()
	}

	if got, want := model.Current().Name, "dependency-01"; got != want {
		t.Fatalf("current dependency = %q, want %q", got, want)
	}

	if got, want := model.SelectedNames(),
		[]string{"dependency-11"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("selected dependencies = %v, want %v", got, want)
	}
}

func TestDependencyMultiSelectRejectsEmptyConfirmation(t *testing.T) {
	model := newDependencyMultiSelect([]dependency.Definition{
		{Name: "windows-exporter"},
	})

	_, err := model.Confirm()

	if err == nil {
		t.Fatal("Confirm() error = nil, want selection-required error")
	}

	if got, want := err.Error(), "select at least one dependency"; got != want {
		t.Fatalf("Confirm() error = %q, want %q", got, want)
	}
}

func TestDependencyMultiSelectConfirmsSelectedNamesInCatalogOrder(
	t *testing.T,
) {
	model := newDependencyMultiSelect([]dependency.Definition{
		{Name: "libre-hardware-monitor"},
		{Name: "windows-exporter"},
		{Name: "future-exporter"},
	})

	model.MoveDown()
	model.MoveDown()
	model.ToggleCurrent() // future-exporter

	model.MoveUp()
	model.MoveUp()
	model.ToggleCurrent() // libre-hardware-monitor

	names, err := model.Confirm()
	if err != nil {
		t.Fatalf("Confirm() error = %v", err)
	}

	want := []string{
		"libre-hardware-monitor",
		"future-exporter",
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("confirmed names = %v, want %v", names, want)
	}
}
