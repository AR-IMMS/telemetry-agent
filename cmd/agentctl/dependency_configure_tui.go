package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

type dependencyConfigureOption struct {
	Definition dependency.Definition
	Enabled    bool
}

type dependencyConfigureTUI struct {
	options   []dependencyConfigureOption
	selection dependencyMultiSelect
	complete  bool
	cancelled bool
}

func newDependencyConfigureTUI(
	options []dependencyConfigureOption,
) dependencyConfigureTUI {
	definitions := make([]dependency.Definition, len(options))
	selection := newDependencyMultiSelect(definitions)

	for index, option := range options {
		definitions[index] = option.Definition
		selection.selected[index] = option.Enabled
	}

	selection.definitions = definitions

	return dependencyConfigureTUI{
		options:   options,
		selection: selection,
	}
}

func (m dependencyConfigureTUI) Init() tea.Cmd {
	return nil
}

func (m dependencyConfigureTUI) Update(
	message tea.Msg,
) (tea.Model, tea.Cmd) {
	key, ok := message.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	if key.Type == tea.KeySpace ||
		(key.Type == tea.KeyRunes &&
			len(key.Runes) == 1 &&
			key.Runes[0] == ' ') {
		m.selection.ToggleCurrent()

		return m, nil
	}

	switch key.Type {
	case tea.KeyUp:
		m.selection.MoveUp()

	case tea.KeyDown:
		m.selection.MoveDown()

	case tea.KeyEnter:
		m.complete = true

		return m, tea.Quit

	case tea.KeyEsc, tea.KeyCtrlC:
		m.cancelled = true

		return m, tea.Quit
	}

	if key.String() == "q" {
		m.cancelled = true

		return m, tea.Quit
	}

	return m, nil
}

func (m dependencyConfigureTUI) DesiredStates() map[string]bool {
	states := make(map[string]bool, len(m.options))

	for index, option := range m.options {
		states[option.Definition.Name] = m.selection.selected[index]
	}

	return states
}

func (m dependencyConfigureTUI) View() string {
	var output strings.Builder

	output.WriteString("Configure managed dependencies\n")
	output.WriteString("Space: toggle  Enter: apply  q/Esc: cancel\n\n")

	for offset, definition := range m.selection.Visible() {
		index := m.selection.firstVisible + offset

		cursor := " "
		if index == m.selection.cursor {
			cursor = ">"
		}

		marker := "○"
		if m.selection.selected[index] {
			marker = "●"
		}

		displayName := definition.DisplayName
		if displayName == "" {
			displayName = definition.Name
		}

		fmt.Fprintf(
			&output,
			"%s %s %s\n",
			cursor,
			marker,
			displayName,
		)
	}

	return output.String()
}
