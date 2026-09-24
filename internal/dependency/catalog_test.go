package dependency

import (
	"context"
	"reflect"
	"testing"
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
		},
		{
			Definition: Definition{
				Name:        "node-exporter",
				DisplayName: "Node Exporter",
				Description: "Collects Linux host metrics.",
				SupportedOS: []string{"linux"},
			},
			Install: testCatalogInstaller,
		},
		{
			Definition: Definition{
				Name:        "libre-hardware-monitor",
				DisplayName: "Libre Hardware Monitor",
				Description: "Collects Windows hardware metrics.",
				SupportedOS: []string{"windows"},
			},
			Install: testCatalogInstaller,
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
