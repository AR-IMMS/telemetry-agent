package librehardwaremonitor

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestRegisterTaskCreatesThenRunsManagedTask(t *testing.T) {
	var commands []processCommand

	err := RegisterTask(
		context.Background(),
		DefaultOptions(),
		testInteractiveUserSID,
		func(
			ctx context.Context,
			command processCommand,
		) error {
			commands = append(commands, command)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("RegisterTask() error = %v", err)
	}

	if len(commands) != 2 {
		t.Fatalf("command count = %d, want 2", len(commands))
	}

	if !reflect.DeepEqual(
		commands[0].Args[:3],
		[]string{"/Create", "/TN", "AR-IMMS-LibreHardwareMonitor"},
	) {
		t.Fatalf("create command = %#v", commands[0].Args)
	}

	if !reflect.DeepEqual(
		commands[1],
		processCommand{
			Executable: "schtasks.exe",
			Args: []string{
				"/Run",
				"/TN",
				"AR-IMMS-LibreHardwareMonitor",
			},
		},
	) {
		t.Fatalf("run command = %#v", commands[1])
	}
}

func TestRegisterTaskDoesNotRunTaskWhenCreationFails(t *testing.T) {
	calls := 0

	err := RegisterTask(
		context.Background(),
		DefaultOptions(),
		testInteractiveUserSID,
		func(
			ctx context.Context,
			command processCommand,
		) error {
			calls++

			return fmt.Errorf("schtasks create failed")
		},
	)

	if err == nil {
		t.Fatal("RegisterTask() error = nil, want create failure")
	}
	if !strings.Contains(err.Error(), "create") {
		t.Fatalf(
			"RegisterTask() error = %v, want create context",
			err,
		)
	}
	if calls != 1 {
		t.Fatalf("process calls = %d, want 1 without /Run", calls)
	}
}
