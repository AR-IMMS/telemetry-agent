package librehardwaremonitor

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestRunOSProcessReturnsExitCodeAndDiagnostics(t *testing.T) {
	t.Setenv("GO_WANT_LHM_PROCESS_HELPER", "1")

	err := runOSProcess(
		context.Background(),
		processCommand{
			Executable: os.Args[0],
			Args: []string{
				"-test.run=^TestRunOSProcessHelper$",
			},
		},
	)

	if err == nil {
		t.Fatal("runOSProcess() error = nil, want helper failure")
	}

	for _, want := range []string{
		"exit status 7",
		"intentional LHM helper failure",
		"arguments",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf(
				"runOSProcess() error = %v, want %q",
				err,
				want,
			)
		}
	}
}

func TestRunOSProcessHelper(t *testing.T) {
	if os.Getenv("GO_WANT_LHM_PROCESS_HELPER") != "1" {
		return
	}

	fmt.Fprint(os.Stderr, "intentional LHM helper failure")
	os.Exit(7)
}

func TestRunOSProcessOutputReturnsStandardOutput(t *testing.T) {
	t.Setenv("GO_WANT_LHM_OUTPUT_HELPER", "1")

	output, err := runOSProcessOutput(
		context.Background(),
		processCommand{
			Executable: os.Args[0],
			Args: []string{
				"-test.run=^TestRunOSProcessOutputHelper$",
			},
		},
	)
	if err != nil {
		t.Fatalf("runOSProcessOutput() error = %v", err)
	}

	if got := string(output); !strings.Contains(got, "LHM output helper") {
		t.Fatalf(
			"runOSProcessOutput() = %q, want helper output",
			got,
		)
	}
}

func TestRunOSProcessOutputHelper(t *testing.T) {
	if os.Getenv("GO_WANT_LHM_OUTPUT_HELPER") != "1" {
		return
	}

	fmt.Fprint(os.Stdout, "LHM output helper")
}
