package nodeexporter

import "fmt"

// requireLinuxRoot rejects installation when the effective user ID is not root.
// The probe is injected so privilege policy can be tested without sudo.
func requireLinuxRoot(effectiveUserID func() int) error {
	if effectiveUserID == nil {
		return fmt.Errorf("Node Exporter effective user ID probe is required")
	}

	if effectiveUserID() != 0 {
		return fmt.Errorf(
			"Node Exporter installation requires a root terminal",
		)
	}

	return nil
}
