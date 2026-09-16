package dependency

import "strings"

// Definition describes one built-in external dependency supported by agentctl.
type Definition struct {
	Name        string
	SupportedOS []string
}

// InstallResult reports the outcome of a dependency installation attempt.
type InstallResult struct {
	Name   string
	Reused bool
}

// SupportsOS reports whether the dependency can run on the supplied OS name.
func (d Definition) SupportsOS(osName string) bool {
	normalizedOS := strings.ToLower(strings.TrimSpace(osName))

	for _, supportedOS := range d.SupportedOS {
		if normalizedOS == strings.ToLower(strings.TrimSpace(supportedOS)) {
			return true
		}
	}

	return false
}
