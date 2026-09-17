package nodeexporter

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestReloadAndRestartSystemdServiceReloadsThenRestarts(t *testing.T) {
	var got []systemdCommand

	err := ReloadAndRestartSystemdService(
		context.Background(),
		"ar-imms-node-exporter.service",
		func(
			ctx context.Context,
			command systemdCommand,
		) error {
			got = append(got, command)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ReloadAndRestartSystemdService() error = %v", err)
	}

	want := []systemdCommand{
		{
			Executable: "systemctl",
			Args:       []string{"daemon-reload"},
		},
		{
			Executable: "systemctl",
			Args:       []string{"restart", "ar-imms-node-exporter.service"},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf(
			"systemd commands = %#v, want %#v",
			got,
			want,
		)
	}
}

func TestReloadAndRestartSystemdServiceStopsWhenReloadFails(t *testing.T) {
	calls := 0

	err := ReloadAndRestartSystemdService(
		context.Background(),
		"ar-imms-node-exporter.service",
		func(
			ctx context.Context,
			command systemdCommand,
		) error {
			calls++

			if !reflect.DeepEqual(command.Args, []string{"daemon-reload"}) {
				t.Fatalf(
					"command args = %#v, want daemon-reload only",
					command.Args,
				)
			}

			return errors.New("intentional daemon reload failure")
		},
	)

	if err == nil {
		t.Fatal("ReloadAndRestartSystemdService() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "daemon-reload") {
		t.Fatalf("error = %v, want daemon-reload context", err)
	}
	if calls != 1 {
		t.Fatalf("systemd calls = %d, want 1", calls)
	}
}

func TestReloadAndRestartSystemdServiceReturnsRestartFailure(t *testing.T) {
	var got []systemdCommand

	err := ReloadAndRestartSystemdService(
		context.Background(),
		"ar-imms-node-exporter.service",
		func(
			ctx context.Context,
			command systemdCommand,
		) error {
			got = append(got, command)

			if command.Args[0] == "restart" {
				return errors.New("intentional restart failure")
			}

			return nil
		},
	)

	if err == nil {
		t.Fatal("ReloadAndRestartSystemdService() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "restart") {
		t.Fatalf("error = %v, want restart context", err)
	}

	want := []systemdCommand{
		{
			Executable: "systemctl",
			Args:       []string{"daemon-reload"},
		},
		{
			Executable: "systemctl",
			Args:       []string{"restart", "ar-imms-node-exporter.service"},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("systemd commands = %#v, want %#v", got, want)
	}
}
