package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

type dependencyMultiSelectTUI struct {
	selection dependencyMultiSelect
	complete  bool
	cancelled bool
	message   string
}

func newDependencyMultiSelectTUI(
	definitions []dependency.Definition,
) dependencyMultiSelectTUI {
	return dependencyMultiSelectTUI{
		selection: newDependencyMultiSelect(definitions),
	}
}

func (m dependencyMultiSelectTUI) Init() tea.Cmd {
	return nil
}

func (m dependencyMultiSelectTUI) Update(
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
		m.message = ""

		return m, nil
	}

	switch key.Type {
	case tea.KeyUp:
		m.selection.MoveUp()

	case tea.KeyDown:
		m.selection.MoveDown()

	case tea.KeyEnter:
		if _, err := m.selection.Confirm(); err != nil {
			m.message = err.Error()

			return m, nil
		}

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

func (m dependencyMultiSelectTUI) View() string {
	var output strings.Builder

	output.WriteString("Select dependencies to install\n")
	output.WriteString("Space: toggle  Enter: confirm  q/Esc: cancel\n\n")

	for offset, definition := range m.selection.Visible() {
		index := m.selection.firstVisible + offset

		cursor := " "
		if index == m.selection.cursor {
			cursor = ">"
		}

		checkbox := "[ ]"
		if m.selection.selected[index] {
			checkbox = "[x]"
		}

		fmt.Fprintf(
			&output,
			"%s %s %s\n",
			cursor,
			checkbox,
			definition.DisplayName,
		)
	}

	if m.message != "" {
		fmt.Fprintf(&output, "\n%s\n", m.message)
	}

	return output.String()
}
