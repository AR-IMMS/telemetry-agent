package nodeexporter

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestEnableAndStartSystemdServiceReloadsThenEnablesService(
	t *testing.T,
) {
	var gotCommands []systemdCommand

	run := func(
		ctx context.Context,
		command systemdCommand,
	) error {
		gotCommands = append(gotCommands, command)
		return nil
	}

	err := EnableAndStartSystemdService(
		context.Background(),
		"ar-imms-node-exporter.service",
		run,
	)
	if err != nil {
		t.Fatalf("EnableAndStartSystemdService() error = %v", err)
	}

	wantCommands := []systemdCommand{
		{
			Executable: "systemctl",
			Args:       []string{"daemon-reload"},
		},
		{
			Executable: "systemctl",
			Args: []string{
				"enable",
				"--now",
				"ar-imms-node-exporter.service",
			},
		},
	}

	if !reflect.DeepEqual(gotCommands, wantCommands) {
		t.Fatalf(
			"systemd commands = %#v, want %#v",
			gotCommands,
			wantCommands,
		)
	}
}

func TestEnableAndStartSystemdServiceStopsWhenReloadFails(
	t *testing.T,
) {
	var gotCommands []systemdCommand

	run := func(
		ctx context.Context,
		command systemdCommand,
	) error {
		gotCommands = append(gotCommands, command)

		return errors.New("systemd manager is unavailable")
	}

	err := EnableAndStartSystemdService(
		context.Background(),
		"ar-imms-node-exporter.service",
		run,
	)

	if err == nil {
		t.Fatal("EnableAndStartSystemdService() error = nil, want an error")
	}
	if !strings.Contains(err.Error(), "systemd manager is unavailable") {
		t.Fatalf(
			"EnableAndStartSystemdService() error = %v, want runner error",
			err,
		)
	}

	wantCommands := []systemdCommand{
		{
			Executable: "systemctl",
			Args:       []string{"daemon-reload"},
		},
	}

	if !reflect.DeepEqual(gotCommands, wantCommands) {
		t.Fatalf(
			"systemd commands = %#v, want %#v",
			gotCommands,
			wantCommands,
		)
	}
}
