package dependency

import "fmt"

// administratorProbe reports whether the current process has Administrator
// privileges on Windows.
type administratorProbe func() (bool, error)

// requireWindowsAdministrator rejects installation before any machine-wide
// changes when the current process is not elevated.
func requireWindowsAdministrator(
	probe administratorProbe,
) error {
	if probe == nil {
		return fmt.Errorf("Windows Administrator privilege probe is required")
	}

	elevated, err := probe()
	if err != nil {
		return fmt.Errorf(
			"determine Windows Administrator privileges: %w",
			err,
		)
	}
	if !elevated {
		return fmt.Errorf(
			"Windows Exporter installation requires an Administrator terminal",
		)
	}

	return nil
}
