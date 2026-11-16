package dependency

import (
	"context"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

func TestServiceTeardownRoutesSupportedDependency(t *testing.T) {
	var gotAction agentstate.TeardownAction
	var gotResources []agentstate.OwnedResource

	catalog, err := NewCatalog([]Integration{
		{
			Definition: Definition{
				Name:        "node-exporter",
				DisplayName: "Node Exporter",
				SupportedOS: []string{"linux"},
			},
			Install: func(
				context.Context,
			) (InstallResult, error) {
				return InstallResult{}, nil
			},
			Inspect: func(
				context.Context,
			) (Inspection, error) {
				return Inspection{}, nil
			},
			Teardown: func(
				ctx context.Context,
				action agentstate.TeardownAction,
				resources []agentstate.OwnedResource,
			) error {
				gotAction = action
				gotResources = resources

				return nil
			},
		},
	})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}

	service := Service{
		Catalog: catalog,
		OS:      "linux",
	}

	resources := []agentstate.OwnedResource{
		{
			Kind:       "systemd-service",
			Identifier: "ar-imms-node-exporter.service",
		},
	}

	if err := service.Teardown(
		context.Background(),
		"node-exporter",
		agentstate.TeardownActionDisable,
		resources,
	); err != nil {
		t.Fatalf("Teardown() error = %v", err)
	}

	if gotAction != agentstate.TeardownActionDisable {
		t.Fatalf(
			"teardown action = %q, want disable",
			gotAction,
		)
	}
	if len(gotResources) != 1 ||
		gotResources[0].Identifier !=
			"ar-imms-node-exporter.service" {
		t.Fatalf("teardown resources = %#v", gotResources)
	}
}
