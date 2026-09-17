package dependency

import "strings"

// Registry provides lookup for dependencies compiled into agentctl.
type Registry struct {
	definitions map[string]Definition
}

var libreHardwareMonitorDefinition = Definition{
	Name:        "libre-hardware-monitor",
	SupportedOS: []string{"windows"},
}

// DefaultRegistry contains dependencies supported by this Agent version.
func DefaultRegistry() Registry {
	return Registry{
		definitions: map[string]Definition{
			"windows-exporter":       windowsExporterDefinition,
			"libre-hardware-monitor": libreHardwareMonitorDefinition,
			"node-exporter": {
				Name:        "node-exporter",
				SupportedOS: []string{"linux"},
			},
		},
	}
}

// Find returns a dependency definition by its stable CLI name.
func (r Registry) Find(name string) (Definition, bool) {
	definition, found := r.definitions[strings.ToLower(strings.TrimSpace(name))]

	return definition, found
}
