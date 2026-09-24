package nodeexporter

import (
	"context"
	"reflect"
	"testing"
)

func TestSystemdUnitEnabledReadsEnabledUnitFileState(
	t *testing.T,
) {
	enabled, err := systemdUnitEnabled(
		context.Background(),
		"ar-imms-node-exporter.service",
		func(
			ctx context.Context,
			command systemdCommand,
		) ([]byte, error) {
			if command.Executable != "systemctl" {
				t.Fatalf(
					"executable = %q, want systemctl",
					command.Executable,
				)
			}

			wantArgs := []string{
				"show",
				"--property=UnitFileState",
				"--value",
				"ar-imms-node-exporter.service",
			}
			if !reflect.DeepEqual(command.Args, wantArgs) {
				t.Fatalf(
					"arguments = %#v, want %#v",
					command.Args,
					wantArgs,
				)
			}

			return []byte("enabled\n"), nil
		},
	)
	if err != nil {
		t.Fatalf("systemdUnitEnabled() error = %v", err)
	}
	if !enabled {
		t.Fatal("systemdUnitEnabled() = false, want true")
	}
}
