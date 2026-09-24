package main

import (
	"bytes"
	"context"
	"io"
	"reflect"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

func TestRunDependencyInstallWithoutNameSelectsAvailableDependency(
	t *testing.T,
) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	installedName := ""
	selectorCalled := false

	exitCode := runDependency(
		context.Background(),
		[]string{"install"},
		&stdout,
		&stderr,
		dependencies{
			listDependencies: func(
				context.Context,
			) ([]dependency.Definition, error) {
				return []dependency.Definition{
					{
						Name:        "windows-exporter",
						DisplayName: "Windows Exporter",
						Description: "Collects Windows host metrics.",
						SupportedOS: []string{"windows"},
					},
				}, nil
			},
			selectDependencies: func(
				ctx context.Context,
				definitions []dependency.Definition,
				output io.Writer,
			) ([]string, error) {
				selectorCalled = true

				if len(definitions) != 1 ||
					definitions[0].Name != "windows-exporter" {
					t.Fatalf(
						"selectable definitions = %#v, want Windows Exporter",
						definitions,
					)
				}

				return []string{"windows-exporter"}, nil
			},
			installDependency: func(
				ctx context.Context,
				name string,
			) (dependency.InstallResult, error) {
				installedName = name

				return dependency.InstallResult{
					Name: name,
				}, nil
			},
		},
	)

	if exitCode != 0 {
		t.Fatalf(
			"runDependency(install) exit code = %d, want 0",
			exitCode,
		)
	}

	if !selectorCalled {
		t.Fatal("dependency selector was not called")
	}

	if installedName != "windows-exporter" {
		t.Fatalf(
			"installed dependency = %q, want windows-exporter",
			installedName,
		)
	}

	if got, want := stdout.String(),
		"Dependency installed: windows-exporter\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}

	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunDependencyInstallWithoutNameReportsCancellation(
	t *testing.T,
) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	installCalled := false

	exitCode := runDependency(
		context.Background(),
		[]string{"install"},
		&stdout,
		&stderr,
		dependencies{
			listDependencies: func(
				context.Context,
			) ([]dependency.Definition, error) {
				return []dependency.Definition{
					{
						Name:        "windows-exporter",
						DisplayName: "Windows Exporter",
						Description: "Collects Windows host metrics.",
					},
				}, nil
			},
			selectDependencies: func(
				context.Context,
				[]dependency.Definition,
				io.Writer,
			) ([]string, error) {
				return nil, errDependencySelectionCancelled
			},
			installDependency: func(
				context.Context,
				string,
			) (dependency.InstallResult, error) {
				installCalled = true

				return dependency.InstallResult{}, nil
			},
		},
	)

	if exitCode != 0 {
		t.Fatalf(
			"runDependency(install) exit code = %d, want 0",
			exitCode,
		)
	}

	if installCalled {
		t.Fatal("installer was called after selection cancellation")
	}

	if got, want := stdout.String(),
		"Dependency installation cancelled.\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}

	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunDependencyInstallWithoutNameInstallsSelectedDependenciesInOrder(
	t *testing.T,
) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	var installed []string

	exitCode := runDependency(
		context.Background(),
		[]string{"install"},
		&stdout,
		&stderr,
		dependencies{
			listDependencies: func(
				context.Context,
			) ([]dependency.Definition, error) {
				return []dependency.Definition{
					{Name: "libre-hardware-monitor"},
					{Name: "windows-exporter"},
				}, nil
			},
			selectDependencies: func(
				context.Context,
				[]dependency.Definition,
				io.Writer,
			) ([]string, error) {
				return []string{
					"libre-hardware-monitor",
					"windows-exporter",
				}, nil
			},
			installDependency: func(
				ctx context.Context,
				name string,
			) (dependency.InstallResult, error) {
				installed = append(installed, name)

				return dependency.InstallResult{
					Name: name,
				}, nil
			},
		},
	)

	if exitCode != 0 {
		t.Fatalf(
			"runDependency(install) exit code = %d, want 0",
			exitCode,
		)
	}

	wantInstalled := []string{
		"libre-hardware-monitor",
		"windows-exporter",
	}
	if !reflect.DeepEqual(installed, wantInstalled) {
		t.Fatalf(
			"installed dependencies = %v, want %v",
			installed,
			wantInstalled,
		)
	}

	const wantStdout = `Dependency installed: libre-hardware-monitor
Dependency installed: windows-exporter
`

	if stdout.String() != wantStdout {
		t.Fatalf("stdout = %q, want %q", stdout.String(), wantStdout)
	}

	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}
