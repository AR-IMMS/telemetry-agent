package main

import (
	"fmt"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

const dependencyMultiSelectViewportSize = 10

type dependencyMultiSelect struct {
	definitions  []dependency.Definition
	cursor       int
	firstVisible int
	selected     map[int]bool
}

func newDependencyMultiSelect(
	definitions []dependency.Definition,
) dependencyMultiSelect {
	return dependencyMultiSelect{
		definitions: definitions,
		selected:    make(map[int]bool),
	}
}

func (m *dependencyMultiSelect) MoveDown() {
	if m.cursor >= len(m.definitions)-1 {
		return
	}

	m.cursor++

	if m.cursor >= m.firstVisible+dependencyMultiSelectViewportSize {
		m.firstVisible = m.cursor -
			dependencyMultiSelectViewportSize + 1
	}
}

func (m dependencyMultiSelect) Current() dependency.Definition {
	if len(m.definitions) == 0 {
		return dependency.Definition{}
	}

	return m.definitions[m.cursor]
}

func (m dependencyMultiSelect) Visible() []dependency.Definition {
	end := m.firstVisible + dependencyMultiSelectViewportSize
	if end > len(m.definitions) {
		end = len(m.definitions)
	}

	return m.definitions[m.firstVisible:end]
}

func (m *dependencyMultiSelect) MoveUp() {
	if m.cursor == 0 {
		return
	}

	m.cursor--

	if m.cursor < m.firstVisible {
		m.firstVisible = m.cursor
	}
}

func (m *dependencyMultiSelect) ToggleCurrent() {
	if len(m.definitions) == 0 {
		return
	}

	m.selected[m.cursor] = !m.selected[m.cursor]
}

func (m dependencyMultiSelect) SelectedNames() []string {
	names := make([]string, 0, len(m.selected))

	for index, definition := range m.definitions {
		if m.selected[index] {
			names = append(names, definition.Name)
		}
	}

	return names
}

func (m dependencyMultiSelect) Confirm() ([]string, error) {
	names := m.SelectedNames()
	if len(names) == 0 {
		return nil, fmt.Errorf("select at least one dependency")
	}

	return names, nil
}
