package config

import "github.com/ar-imms/telemetry-agent/internal/identity"

// Layer identifies one ordered YAML configuration fragment.
type Layer struct {
	Name string
	Path string
}

// InlineLayer identifies an in-memory document merged after file-backed layers.
type InlineLayer struct {
	Name     string
	Document map[string]any
}

// RenderInput supplies configuration layers and explicit substitution values.
type RenderInput struct {
	Platform     identity.PlatformInfo
	Layers       []Layer
	InlineLayers []InlineLayer
	Values       map[string]string
}
