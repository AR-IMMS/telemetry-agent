package main

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

func TestDependencyConfigureTUITogglesDesiredStatesAndConfirms(
	t *testing.T,
) {
	model := newDependencyConfigureTUI(
		[]dependencyConfigureOption{
			{
				Definition: dependency.Definition{
					Name:        "windows-exporter",
					DisplayName: "Windows Exporter",
				},
				Enabled: true,
			},
			{
				Definition: dependency.Definition{
					Name:        "libre-hardware-monitor",
					DisplayName: "Libre Hardware Monitor",
				},
				Enabled: false,
			},
		},
	)

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	updated := next.(dependencyConfigureTUI)

	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyDown})
	updated = next.(dependencyConfigureTUI)

	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeySpace})
	updated = next.(dependencyConfigureTUI)

	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated = next.(dependencyConfigureTUI)

	if !updated.complete {
		t.Fatal("model complete = false, want true after Enter")
	}

	want := map[string]bool{
		"windows-exporter":       false,
		"libre-hardware-monitor": true,
	}

	if got := updated.DesiredStates(); !reflect.DeepEqual(got, want) {
		t.Fatalf("desired states = %#v, want %#v", got, want)
	}
}

func TestDependencyConfigureTUIRendersEnabledAndDisabledMarkers(
	t *testing.T,
) {
	model := newDependencyConfigureTUI(
		[]dependencyConfigureOption{
			{
				Definition: dependency.Definition{
					Name:        "windows-exporter",
					DisplayName: "Windows Exporter",
				},
				Enabled: true,
			},
			{
				Definition: dependency.Definition{
					Name:        "libre-hardware-monitor",
					DisplayName: "Libre Hardware Monitor",
				},
				Enabled: false,
			},
		},
	)

	view := model.View()

	for _, want := range []string{
		"Configure managed dependencies",
		"● Windows Exporter",
		"○ Libre Hardware Monitor",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() = %q, want %q", view, want)
		}
	}
}
