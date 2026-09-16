package dependency

import "testing"

func TestDefaultRegistryFindsWindowsExporter(t *testing.T) {
	definition, found := DefaultRegistry().Find("windows-exporter")

	if !found {
		t.Fatal("DefaultRegistry().Find(windows-exporter) found = false")
	}

	if definition.Name != "windows-exporter" {
		t.Fatalf(
			"definition.Name = %q, want windows-exporter",
			definition.Name,
		)
	}

	if !definition.SupportsOS("windows") {
		t.Fatal("windows-exporter does not support windows")
	}
}
