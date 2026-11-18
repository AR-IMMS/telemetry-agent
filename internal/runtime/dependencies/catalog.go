package dependency

import (
	"fmt"
	"sort"
	"strings"
)

// EnabledSet contains normalized dependency names that are enabled in desired
// state. It does not report installation health or resource ownership.
type EnabledSet map[string]struct{}

// Catalog provides one source of truth for built-in dependency definitions
// and their concrete installers.
type Catalog struct {
	integrations map[string]Integration
}

// NewCatalog creates a validated catalog from built-in dependency integrations.
func NewCatalog(integrations []Integration) (Catalog, error) {
	catalog := Catalog{
		integrations: make(map[string]Integration, len(integrations)),
	}

	for _, integration := range integrations {
		name := strings.ToLower(
			strings.TrimSpace(integration.Definition.Name),
		)

		if name == "" {
			return Catalog{}, fmt.Errorf(
				"dependency catalog integration name is required",
			)
		}
		if len(integration.Definition.SupportedOS) == 0 {
			return Catalog{}, fmt.Errorf(
				"dependency catalog integration %q must support at least one operating system",
				name,
			)
		}
		if integration.Install == nil {
			return Catalog{}, fmt.Errorf(
				"dependency catalog integration %q installer is required",
				name,
			)
		}
		if integration.Inspect == nil {
			return Catalog{}, fmt.Errorf(
				"dependency catalog integration %q inspector is required",
				name,
			)
		}
		if _, exists := catalog.integrations[name]; exists {
			return Catalog{}, fmt.Errorf(
				"dependency catalog integration %q is registered more than once",
				name,
			)
		}
		if _, exists := catalog.integrations[name]; exists {
			return Catalog{}, fmt.Errorf(
				"dependency catalog integration %q is registered more than once",
				name,
			)
		}

		integration.Definition.Name = name
		catalog.integrations[name] = integration
	}

	return catalog, nil
}

// Find returns one integration by its stable CLI name.
func (c Catalog) Find(name string) (Integration, bool) {
	normalizedName := strings.ToLower(strings.TrimSpace(name))

	integration, found := c.integrations[normalizedName]

	return integration, found
}

// List returns definitions supported by the supplied operating system,
// ordered by display name for predictable CLI and picker output.
func (c Catalog) List(osName string) []Definition {
	definitions := make([]Definition, 0, len(c.integrations))

	for _, integration := range c.integrations {
		if integration.Definition.SupportsOS(osName) {
			definitions = append(definitions, integration.Definition)
		}
	}

	sort.Slice(definitions, func(left, right int) bool {
		return strings.ToLower(definitions[left].DisplayName) <
			strings.ToLower(definitions[right].DisplayName)
	})

	return definitions
}

// CollectorReceivers returns configured Collector receiver names for enabled
// dependencies supported by the supplied operating system.
func (c Catalog) CollectorReceivers(
	osName string,
	enabledDependencies EnabledSet,
) ([]string, error) {
	definitions := c.List(osName)
	receivers := make([]string, 0, len(definitions))

	for _, definition := range definitions {
		if _, enabled := enabledDependencies[definition.Name]; !enabled {
			continue
		}

		integration, found := c.Find(definition.Name)
		if !found {
			return nil, fmt.Errorf(
				"dependency catalog integration %q is missing",
				definition.Name,
			)
		}

		receiver := strings.TrimSpace(integration.CollectorReceiver)
		if receiver == "" {
			return nil, fmt.Errorf(
				"dependency %q Collector receiver is required",
				definition.Name,
			)
		}

		receivers = append(receivers, receiver)
	}

	return receivers, nil
}
