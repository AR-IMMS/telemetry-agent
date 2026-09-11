package config

import "github.com/ar-imms/telemetry-agent/internal/identity"

// Layer identifies one ordered YAML configuration fragment.
type Layer struct {
	Name string
	Path string
}

// RenderInput supplies configuration layers and explicit substitution values.
type RenderInput struct {
	Platform identity.PlatformInfo
	Layers   []Layer
	Values   map[string]string
}
