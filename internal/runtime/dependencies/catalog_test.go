package dependency

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

func TestCatalogListReturnsSupportedDefinitionsSortedByDisplayName(
	t *testing.T,
) {
	catalog, err := NewCatalog([]Integration{
		{
			Definition: Definition{
				Name:        "windows-exporter",
				DisplayName: "Windows Exporter",
				Description: "Collects Windows host metrics.",
				SupportedOS: []string{"windows"},
			},
			Install: testCatalogInstaller,
			Inspect: testCatalogInspector,
		},
		{
			Definition: Definition{
				Name:        "node-exporter",
				DisplayName: "Node Exporter",
				Description: "Collects Linux host metrics.",
				SupportedOS: []string{"linux"},
			},
			Install: testCatalogInstaller,
			Inspect: testCatalogInspector,
		},
		{
			Definition: Definition{
				Name:        "libre-hardware-monitor",
				DisplayName: "Libre Hardware Monitor",
				Description: "Collects Windows hardware metrics.",
				SupportedOS: []string{"windows"},
			},
			Install: testCatalogInstaller,
			Inspect: testCatalogInspector,
		},
	})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}

	got := catalog.List("windows")

	want := []Definition{
		{
			Name:        "libre-hardware-monitor",
			DisplayName: "Libre Hardware Monitor",
			Description: "Collects Windows hardware metrics.",
			SupportedOS: []string{"windows"},
		},
		{
			Name:        "windows-exporter",
			DisplayName: "Windows Exporter",
			Description: "Collects Windows host metrics.",
			SupportedOS: []string{"windows"},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Catalog.List() = %#v, want %#v", got, want)
	}
}

func testCatalogInspector(
	context.Context,
) (Inspection, error) {
	return Inspection{}, nil
}

func testCatalogInstaller(
	context.Context,
) (InstallResult, error) {
	return InstallResult{}, nil
}

func TestNewCatalogRejectsInvalidIntegrations(t *testing.T) {
	testIntegration := func(name string) Integration {
		return Integration{
			Definition: Definition{
				Name:        name,
				DisplayName: "Test Exporter",
				Description: "Test dependency.",
				SupportedOS: []string{"windows"},
			},
			Install: testCatalogInstaller,
		}
	}

	tests := []struct {
		name         string
		integrations []Integration
	}{
		{
			name: "missing stable name",
			integrations: []Integration{
				testIntegration(""),
			},
		},
		{
			name: "missing supported operating systems",
			integrations: []Integration{
				{
					Definition: Definition{
						Name:        "test-exporter",
						DisplayName: "Test Exporter",
						Description: "Test dependency.",
					},
					Install: testCatalogInstaller,
				},
			},
		},
		{
			name: "missing installer",
			integrations: []Integration{
				{
					Definition: Definition{
						Name:        "test-exporter",
						DisplayName: "Test Exporter",
						Description: "Test dependency.",
						SupportedOS: []string{"windows"},
					},
				},
			},
		},
		{
			name: "duplicate normalized names",
			integrations: []Integration{
				testIntegration("node-exporter"),
				testIntegration(" Node-Exporter "),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewCatalog(test.integrations)

			if err == nil {
				t.Fatal("NewCatalog() error = nil, want invalid catalog error")
			}
		})
	}
}

func TestCatalogFindNormalizesDependencyName(t *testing.T) {
	catalog, err := NewCatalog([]Integration{
		{
			Definition: Definition{
				Name:        "node-exporter",
				DisplayName: "Node Exporter",
				Description: "Collects Linux host metrics.",
				SupportedOS: []string{"linux"},
			},
			Install: testCatalogInstaller,
			Inspect: testCatalogInspector,
		},
	})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}

	integration, found := catalog.Find(" Node-Exporter ")

	if !found {
		t.Fatal("Catalog.Find() found = false, want true")
	}

	want := Definition{
		Name:        "node-exporter",
		DisplayName: "Node Exporter",
		Description: "Collects Linux host metrics.",
		SupportedOS: []string{"linux"},
	}

	if !reflect.DeepEqual(integration.Definition, want) {
		t.Fatalf(
			"Catalog.Find() definition = %#v, want %#v",
			integration.Definition,
			want,
		)
	}
	if integration.Install == nil {
		t.Fatal("Catalog.Find() integration installer = nil")
	}
}

func TestNewCatalogRejectsIntegrationWithoutInspector(
	t *testing.T,
) {
	_, err := NewCatalog([]Integration{
		{
			Definition: Definition{
				Name:        "windows-exporter",
				SupportedOS: []string{"windows"},
			},
			Install: func(
				context.Context,
			) (InstallResult, error) {
				return InstallResult{}, nil
			},
		},
	})

	if err == nil {
		t.Fatal("NewCatalog() error = nil, want missing-inspector error")
	}
	if !strings.Contains(err.Error(), "inspector is required") {
		t.Fatalf(
			"NewCatalog() error = %v, want missing-inspector error",
			err,
		)
	}
}

func TestCatalogReturnsReceiverForEnabledCurrentPlatformDependency(
	t *testing.T,
) {
	catalog, err := NewCatalog([]Integration{
		{
			Definition: Definition{
				Name:        "node-exporter",
				DisplayName: "Node Exporter",
				SupportedOS: []string{"linux"},
			},
			CollectorReceiver: "prometheus/node_exporter",
			Install:           testCatalogInstaller,
			Inspect:           testCatalogInspector,
		},
		{
			Definition: Definition{
				Name:        "windows-exporter",
				DisplayName: "Windows Exporter",
				SupportedOS: []string{"windows"},
			},
			CollectorReceiver: "prometheus/windows_exporter",
			Install:           testCatalogInstaller,
			Inspect:           testCatalogInspector,
		},
	})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}

	got, err := catalog.CollectorReceivers(
		"linux",
		agentstate.State{
			Dependencies: map[string]agentstate.DependencyState{
				"node-exporter": {
					Enabled: true,
				},
				"windows-exporter": {
					Enabled: true,
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("CollectorReceivers() error = %v", err)
	}

	want := []string{"prometheus/node_exporter"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CollectorReceivers() = %#v, want %#v", got, want)
	}
}
