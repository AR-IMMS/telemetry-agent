package config

import "github.com/ar-imms/telemetry-agent/internal/identity"

type Layer struct {
	Name string
	Path string
}
type RenderInput struct {
	Platform identity.PlatformInfo
	Layers   []Layer
	Values   map[string]string
}
