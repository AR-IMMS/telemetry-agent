package main

import "github.com/ar-imms/telemetry-agent/internal/runtime/dependencies"

type dependencyConfigureOption struct {
	Definition dependency.Definition
	Enabled    bool
}

type dependencyConfigureSelection struct {
	options       []dependencyConfigureOption
	desiredStates map[string]bool
	complete      bool
	cancelled     bool
}

func newDependencyConfigureSelection(options []dependencyConfigureOption) dependencyConfigureSelection {
	selection := dependencyConfigureSelection{
		options:       append([]dependencyConfigureOption(nil), options...),
		desiredStates: make(map[string]bool, len(options)),
	}
	for _, option := range options {
		selection.desiredStates[option.Definition.Name] = option.Enabled
	}
	return selection
}

func (s dependencyConfigureSelection) DesiredStates() map[string]bool {
	states := make(map[string]bool, len(s.desiredStates))
	for name, enabled := range s.desiredStates {
		states[name] = enabled
	}
	return states
}
