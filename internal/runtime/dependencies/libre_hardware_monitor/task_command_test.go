package librehardwaremonitor

import (
	"reflect"
	"testing"
)

func TestBuildTaskCreateCommandRegistersManagedTaskXML(t *testing.T) {
	command, err := buildTaskCreateCommand(
		"AR-IMMS-LibreHardwareMonitor",
		`C:\Temp\ar-imms-lhm-task.xml`,
	)
	if err != nil {
		t.Fatalf("buildTaskCreateCommand() error = %v", err)
	}

	if command.Executable != "schtasks.exe" {
		t.Fatalf(
			"Executable = %q, want schtasks.exe",
			command.Executable,
		)
	}

	wantArgs := []string{
		"/Create",
		"/TN",
		"AR-IMMS-LibreHardwareMonitor",
		"/XML",
		`C:\Temp\ar-imms-lhm-task.xml`,
		"/F",
	}

	if !reflect.DeepEqual(command.Args, wantArgs) {
		t.Fatalf(
			"Args = %#v, want %#v",
			command.Args,
			wantArgs,
		)
	}
}

func TestBuildTaskRunCommandStartsManagedTask(t *testing.T) {
	command, err := buildTaskRunCommand(
		"AR-IMMS-LibreHardwareMonitor",
	)
	if err != nil {
		t.Fatalf("buildTaskRunCommand() error = %v", err)
	}

	if command.Executable != "schtasks.exe" {
		t.Fatalf(
			"Executable = %q, want schtasks.exe",
			command.Executable,
		)
	}

	wantArgs := []string{
		"/Run",
		"/TN",
		"AR-IMMS-LibreHardwareMonitor",
	}

	if !reflect.DeepEqual(command.Args, wantArgs) {
		t.Fatalf(
			"Args = %#v, want %#v",
			command.Args,
			wantArgs,
		)
	}
}
