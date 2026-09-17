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

func TestDefaultRegistryFindsNodeExporter(t *testing.T) {
	registry := DefaultRegistry()

	definition, found := registry.Find("node-exporter")

	if !found {
		t.Fatal("DefaultRegistry() does not contain node-exporter")
	}

	if definition.Name != "node-exporter" {
		t.Fatalf(
			"definition name = %q, want node-exporter",
			definition.Name,
		)
	}

	if len(definition.SupportedOS) != 1 ||
		definition.SupportedOS[0] != "linux" {
		t.Fatalf(
			"supported operating systems = %v, want [linux]",
			definition.SupportedOS,
		)
	}
}
