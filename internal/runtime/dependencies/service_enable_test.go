package dependency

import (
	"context"
	"reflect"
	"testing"

	dependencydomain "github.com/ar-imms/telemetry-agent/internal/runtime/dependencies/domain"
	servicedomain "github.com/ar-imms/telemetry-agent/internal/runtime/service/domain"
)

func TestServiceEnableRoutesOwnedResources(
	t *testing.T,
) {
	ctx := context.Background()
	resources := []servicedomain.OwnedResource{
		{
			Kind:       "systemd-unit",
			Identifier: "/etc/systemd/system/ar-imms-node-exporter.service",
		},
	}

	calls := 0

	catalog, err := NewCatalog([]Integration{
		{
			Definition: Definition{
				Name:        "node-exporter",
				SupportedOS: []string{"linux"},
			},
			Install: func(context.Context) (InstallResult, error) {
				return InstallResult{}, nil
			},
			Inspect: func(context.Context) (Inspection, error) {
				return Inspection{}, nil
			},
			Teardown: func(
				context.Context,
				dependencydomain.TeardownAction,
				[]servicedomain.OwnedResource,
			) error {
				return nil
			},
			Enable: func(
				gotContext context.Context,
				gotResources []servicedomain.OwnedResource,
			) error {
				calls++

				if gotContext != ctx {
					t.Fatal("enable context does not match")
				}
				if !reflect.DeepEqual(gotResources, resources) {
					t.Fatalf(
						"enable resources = %#v, want %#v",
						gotResources,
						resources,
					)
				}

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

	if err := service.Enable(ctx, "node-exporter", resources); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("enable calls = %d, want 1", calls)
	}
}

func TestServiceEnableRejectsMissingEnabler(
	t *testing.T,
) {
	catalog, err := NewCatalog([]Integration{
		{
			Definition: Definition{
				Name:        "node-exporter",
				SupportedOS: []string{"linux"},
			},
			Install: func(context.Context) (InstallResult, error) {
				return InstallResult{}, nil
			},
			Inspect: func(context.Context) (Inspection, error) {
				return Inspection{}, nil
			},
			Teardown: func(
				context.Context,
				dependencydomain.TeardownAction,
				[]servicedomain.OwnedResource,
			) error {
				return nil
			},
		},
	})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}

	err = (Service{
		Catalog: catalog,
		OS:      "linux",
	}).Enable(
		context.Background(),
		"node-exporter",
		[]servicedomain.OwnedResource{
			{
				Kind:       "systemd-unit",
				Identifier: "/etc/systemd/system/ar-imms-node-exporter.service",
			},
		},
	)

	if err == nil {
		t.Fatal("Enable() error = nil, want missing-enabler error")
	}
}
