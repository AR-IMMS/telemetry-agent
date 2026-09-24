package main

import (
	"reflect"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

func TestDependencyMultiSelectTUITogglesAndConfirmsSelection(
	t *testing.T,
) {
	model := newDependencyMultiSelectTUI([]dependency.Definition{
		{Name: "windows-exporter"},
	})

	next, _ := model.Update(
		tea.KeyMsg{Type: tea.KeySpace},
	)

	updated, ok := next.(dependencyMultiSelectTUI)
	if !ok {
		t.Fatalf(
			"updated model type = %T, want dependencyMultiSelectTUI",
			next,
		)
	}

	if got, want := updated.selection.SelectedNames(),
		[]string{"windows-exporter"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("selected names = %v, want %v", got, want)
	}

	next, _ = updated.Update(
		tea.KeyMsg{Type: tea.KeyEnter},
	)

	updated, ok = next.(dependencyMultiSelectTUI)
	if !ok {
		t.Fatalf(
			"confirmed model type = %T, want dependencyMultiSelectTUI",
			next,
		)
	}

	if !updated.complete {
		t.Fatal("model complete = false, want true after Enter")
	}
}

func TestDependencyMultiSelectTUITogglesRuneSpace(
	t *testing.T,
) {
	model := newDependencyMultiSelectTUI(
		[]dependency.Definition{
			{
				Name:        "windows-exporter",
				DisplayName: "Windows Exporter",
			},
		},
	)

	next, _ := model.Update(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune{' '},
	})

	updated := next.(dependencyMultiSelectTUI)

	if !updated.selection.selected[0] {
		t.Fatal("rune Space did not select the current dependency")
	}
}
