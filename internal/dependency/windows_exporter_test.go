package dependency

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

func TestWindowsExporterOptionsValidateRejectsMissingRequiredValues(
	t *testing.T,
) {
	tests := []struct {
		name    string
		options WindowsExporterOptions
		want    string
	}{
		{
			name: "missing installation directory",
			options: WindowsExporterOptions{
				ConfigPath:    "C:/ProgramData/AR-IMMS/windows-exporter/config.yaml",
				ListenAddress: "127.0.0.1:9182",
			},
			want: "installation directory",
		},
		{
			name: "missing configuration path",
			options: WindowsExporterOptions{
				InstallDir:    "C:/Program Files/windows_exporter",
				ListenAddress: "127.0.0.1:9182",
			},
			want: "configuration path",
		},
		{
			name: "missing listen address",
			options: WindowsExporterOptions{
				InstallDir: "C:/Program Files/windows_exporter",
				ConfigPath: "C:/ProgramData/AR-IMMS/windows-exporter/config.yaml",
			},
			want: "listen address",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.options.Validate()

			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf(
					"Validate() error = %v, want error containing %q",
					err,
					test.want,
				)
			}
		})
	}
}

func TestRenderWindowsExporterConfigUsesLocalMetricsEndpoint(t *testing.T) {
	rendered, err := RenderWindowsExporterConfig(
		WindowsExporterOptions{
			InstallDir:    "C:/Program Files/windows_exporter",
			ConfigPath:    "C:/ProgramData/AR-IMMS/windows-exporter/config.yaml",
			ListenAddress: "127.0.0.1:9182",
		},
	)
	if err != nil {
		t.Fatalf("RenderWindowsExporterConfig() error = %v", err)
	}

	got := string(rendered)

	for _, want := range []string{
		"collectors:",
		`enabled: "[defaults]"`,
		"telemetry:",
		"path: /metrics",
		"web:",
		"listen-address: 127.0.0.1:9182",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf(
				"rendered config = %q, want it to contain %q",
				got,
				want,
			)
		}
	}
}

func TestWriteWindowsExporterConfigPersistsRenderedConfiguration(
	t *testing.T,
) {
	configPath := filepath.Join(
		t.TempDir(),
		"windows-exporter",
		"config.yaml",
	)

	options := WindowsExporterOptions{
		InstallDir:    "C:/Program Files/windows_exporter",
		ConfigPath:    configPath,
		ListenAddress: "127.0.0.1:9182",
	}

	if err := WriteWindowsExporterConfig(options); err != nil {
		t.Fatalf("WriteWindowsExporterConfig() error = %v", err)
	}

	rendered, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	for _, want := range []string{
		`enabled: "[defaults]"`,
		"listen-address: 127.0.0.1:9182",
	} {
		if !strings.Contains(string(rendered), want) {
			t.Fatalf(
				"written config = %q, want it to contain %q",
				rendered,
				want,
			)
		}
	}
}

func TestPinnedWindowsExporterArtifactIsAMD64MSI(t *testing.T) {
	artifact := PinnedWindowsExporterArtifact()

	if artifact.Version != "0.31.8" {
		t.Fatalf("artifact version = %q, want 0.31.8", artifact.Version)
	}
	if artifact.FileName != "windows_exporter-0.31.8-amd64.msi" {
		t.Fatalf("artifact file name = %q", artifact.FileName)
	}
	if artifact.URL != "https://github.com/prometheus-community/windows_exporter/releases/download/v0.31.8/windows_exporter-0.31.8-amd64.msi" {
		t.Fatalf("artifact URL = %q", artifact.URL)
	}
	if artifact.SHA256 != "0aadce6afb20182b678bfca9e8f2e8464ef48c469b28b4cf02e99d82158f5d40" {
		t.Fatalf("artifact SHA-256 = %q", artifact.SHA256)
	}
}

func TestDownloadAndVerifyWindowsExporterArtifactRejectsChecksumMismatch(
	t *testing.T,
) {
	artifact := WindowsExporterArtifact{
		URL:    "https://example.test/windows_exporter.msi",
		SHA256: strings.Repeat("0", 64),
	}

	var destination bytes.Buffer

	err := DownloadAndVerifyWindowsExporterArtifact(
		context.Background(),
		artifact,
		testArtifactDownloader{
			content: []byte("not the expected MSI bytes"),
		},
		&destination,
	)

	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf(
			"DownloadAndVerifyWindowsExporterArtifact() error = %v, want checksum mismatch",
			err,
		)
	}
}

// HELPER
type testArtifactDownloader struct {
	content []byte
	err     error
}

func (d testArtifactDownloader) Download(
	_ context.Context,
	_ string,
	dst io.Writer,
) error {
	if d.err != nil {
		return d.err
	}

	_, err := dst.Write(d.content)

	return err
}

func TestDownloadAndVerifyWindowsExporterArtifactAcceptsVerifiedBytes(
	t *testing.T,
) {
	content := []byte("verified MSI bytes")
	sum := sha256.Sum256(content)

	var destination bytes.Buffer

	err := DownloadAndVerifyWindowsExporterArtifact(
		context.Background(),
		WindowsExporterArtifact{
			URL:    "https://example.test/windows_exporter.msi",
			SHA256: hex.EncodeToString(sum[:]),
		},
		testArtifactDownloader{
			content: content,
		},
		&destination,
	)
	if err != nil {
		t.Fatalf(
			"DownloadAndVerifyWindowsExporterArtifact() error = %v",
			err,
		)
	}
	if !bytes.Equal(destination.Bytes(), content) {
		t.Fatalf(
			"downloaded bytes = %q, want %q",
			destination.Bytes(),
			content,
		)
	}
}

func TestDownloadAndVerifyWindowsExporterArtifactRejectsOversizedDownload(
	t *testing.T,
) {
	var destination bytes.Buffer

	err := DownloadAndVerifyWindowsExporterArtifact(
		context.Background(),
		WindowsExporterArtifact{
			URL:    "https://example.test/windows_exporter.msi",
			SHA256: strings.Repeat("0", 64),
		},
		testArtifactDownloader{
			content: bytes.Repeat([]byte("x"), (32<<20)+1),
		},
		&destination,
	)

	if err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf(
			"DownloadAndVerifyWindowsExporterArtifact() error = %v, want size limit error",
			err,
		)
	}
}

func TestStageWindowsExporterMSICreatesVerifiedPrivateFile(
	t *testing.T,
) {
	content := []byte("verified Windows Exporter MSI")
	sum := sha256.Sum256(content)

	artifact := WindowsExporterArtifact{
		FileName: "windows_exporter-test-amd64.msi",
		URL:      "https://example.test/windows_exporter.msi",
		SHA256:   hex.EncodeToString(sum[:]),
	}

	msiPath, cleanup, err := stageWindowsExporterMSI(
		context.Background(),
		artifact,
		testArtifactDownloader{
			content: content,
		},
	)
	if err != nil {
		t.Fatalf("stageWindowsExporterMSI() error = %v", err)
	}
	defer cleanup()

	if filepath.Base(msiPath) != artifact.FileName {
		t.Fatalf(
			"staged file name = %q, want %q",
			filepath.Base(msiPath),
			artifact.FileName,
		)
	}

	got, err := os.ReadFile(msiPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("staged bytes = %q, want %q", got, content)
	}
}

func TestBuildWindowsExporterMSIInstallCommandUsesAgentOwnedConfig(
	t *testing.T,
) {
	command, err := buildWindowsExporterMSIInstallCommand(
		"C:/temporary/windows_exporter-0.31.8-amd64.msi",
		WindowsExporterOptions{
			InstallDir:    "C:/Program Files/windows_exporter",
			ConfigPath:    "C:/ProgramData/AR-IMMS/windows-exporter/config.yaml",
			ListenAddress: "127.0.0.1:9182",
		},
	)
	if err != nil {
		t.Fatalf("buildWindowsExporterMSIInstallCommand() error = %v", err)
	}

	if command.Executable != "msiexec.exe" {
		t.Fatalf(
			"command executable = %q, want msiexec.exe",
			command.Executable,
		)
	}

	wantArgs := []string{
		"/i",
		"C:/temporary/windows_exporter-0.31.8-amd64.msi",
		"/qn",
		"/norestart",
		"CONFIG_FILE=C:/ProgramData/AR-IMMS/windows-exporter/config.yaml",
		"APPLICATIONFOLDER=C:/Program Files/windows_exporter",
	}

	if !reflect.DeepEqual(command.Args, wantArgs) {
		t.Fatalf(
			"command args = %#v, want %#v",
			command.Args,
			wantArgs,
		)
	}
}

func TestBuildWindowsExporterMSIInstallCommandOmitsDefaultInstallFolder(
	t *testing.T,
) {
	options := DefaultWindowsExporterOptions()

	command, err := buildWindowsExporterMSIInstallCommand(
		`C:\temporary\windows_exporter-0.31.8-amd64.msi`,
		options,
	)
	if err != nil {
		t.Fatalf("buildWindowsExporterMSIInstallCommand() error = %v", err)
	}

	wantArgs := []string{
		"/i",
		`C:\temporary\windows_exporter-0.31.8-amd64.msi`,
		"/qn",
		"/norestart",
		"CONFIG_FILE=" + options.ConfigPath,
	}

	if !reflect.DeepEqual(command.Args, wantArgs) {
		t.Fatalf(
			"command args = %#v, want %#v",
			command.Args,
			wantArgs,
		)
	}
}

func TestRequireWindowsAdministratorRejectsUnelevatedProcess(t *testing.T) {
	err := requireWindowsAdministrator(func() (bool, error) {
		return false, nil
	})

	if err == nil || !strings.Contains(err.Error(), "Administrator") {
		t.Fatalf(
			"requireWindowsAdministrator() error = %v, want Administrator error",
			err,
		)
	}
}

func TestWindowsExporterInstallerStagesAndRunsVerifiedMSI(
	t *testing.T,
) {
	content := []byte("verified Windows Exporter MSI")
	sum := sha256.Sum256(content)

	root := t.TempDir()
	var gotCommand processCommand
	healthChecks := 0

	installer := windowsExporterInstaller{
		startupTimeout: time.Second,
		waitForHealth: func(
			_ context.Context,
			endpoint string,
		) error {
			healthChecks++

			if endpoint != "http://127.0.0.1:9182/health" {
				t.Fatalf("health endpoint = %q", endpoint)
			}

			return nil
		},
		options: WindowsExporterOptions{
			InstallDir:    filepath.Join(root, "install"),
			ConfigPath:    filepath.Join(root, "config", "config.yaml"),
			ListenAddress: "127.0.0.1:9182",
		},
		artifact: WindowsExporterArtifact{
			Version:  "test-version",
			FileName: "windows_exporter-test-amd64.msi",
			URL:      "https://example.test/windows_exporter.msi",
			SHA256:   hex.EncodeToString(sum[:]),
		},
		downloader: testArtifactDownloader{
			content: content,
		},
		administrator: func() (bool, error) {
			return true, nil
		},
		runProcess: func(
			_ context.Context,
			command processCommand,
		) error {
			gotCommand = command
			return nil
		},
	}

	result, err := installer.Install(context.Background())
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	if result.Name != "windows-exporter" {
		t.Fatalf("result name = %q, want windows-exporter", result.Name)
	}
	if result.Reused {
		t.Fatal("result Reused = true, want false")
	}

	wantOwnedResources := []agentstate.OwnedResource{
		{
			Kind:       "windows-service",
			Identifier: windowsExporterServiceName,
		},
		{
			Kind:       "msi-product",
			Identifier: "windows_exporter",
		},
		{
			Kind:       "config-file",
			Identifier: installer.options.ConfigPath,
		},
		{
			Kind:       "directory",
			Identifier: installer.options.InstallDir,
		},
	}

	if !reflect.DeepEqual(result.OwnedResources, wantOwnedResources) {
		t.Fatalf(
			"result owned resources = %#v, want %#v",
			result.OwnedResources,
			wantOwnedResources,
		)
	}

	if gotCommand.Executable != "msiexec.exe" {
		t.Fatalf(
			"process executable = %q, want msiexec.exe",
			gotCommand.Executable,
		)
	}

	configBytes, err := os.ReadFile(installer.options.ConfigPath)
	if err != nil {
		t.Fatalf("ReadFile(config) error = %v", err)
	}
	if !strings.Contains(
		string(configBytes),
		"listen-address: 127.0.0.1:9182",
	) {
		t.Fatalf("written config = %q", configBytes)
	}

	if healthChecks != 1 {
		t.Fatalf("health checks = %d, want 1", healthChecks)
	}
}

func TestRunOSProcessReturnsExitCodeAndDiagnostics(t *testing.T) {
	t.Setenv("GO_WANT_PROCESS_HELPER", "1")

	err := runOSProcess(
		context.Background(),
		processCommand{
			Executable: os.Args[0],
			Args: []string{
				"-test.run=TestRunOSProcessHelper",
			},
		},
	)

	if err == nil {
		t.Fatal("runOSProcess() error = nil, want process failure")
	}
	if !strings.Contains(err.Error(), "exit status 7") {
		t.Fatalf("runOSProcess() error = %v, want exit code", err)
	}
	if !strings.Contains(err.Error(), "intentional helper failure") {
		t.Fatalf("runOSProcess() error = %v, want diagnostics", err)
	}
	if !strings.Contains(
		err.Error(),
		"-test.run=TestRunOSProcessHelper",
	) {
		t.Fatalf(
			"runOSProcess() error = %v, want process arguments",
			err,
		)
	}
}

func TestRunOSProcessHelper(t *testing.T) {
	if os.Getenv("GO_WANT_PROCESS_HELPER") != "1" {
		return
	}

	_, _ = fmt.Fprintln(os.Stderr, "intentional helper failure")
	os.Exit(7)
}

func TestDefaultWindowsExporterOptionsUseAgentOwnedLoopbackConfig(
	t *testing.T,
) {
	options := DefaultWindowsExporterOptions()

	if options.InstallDir != `C:\Program Files\windows_exporter` {
		t.Fatalf("install directory = %q", options.InstallDir)
	}
	if options.ConfigPath != `C:\ProgramData\AR-IMMS\windows-exporter\config.yaml` {
		t.Fatalf("config path = %q", options.ConfigPath)
	}
	if options.ListenAddress != "127.0.0.1:9182" {
		t.Fatalf(
			"listen address = %q, want loopback exporter endpoint",
			options.ListenAddress,
		)
	}
}

func TestRunOSProcessIncludesArgumentsWithoutDiagnostics(
	t *testing.T,
) {
	t.Setenv("GO_WANT_SILENT_PROCESS_HELPER", "1")

	err := runOSProcess(
		context.Background(),
		processCommand{
			Executable: os.Args[0],
			Args: []string{
				"-test.run=TestRunOSProcessSilentHelper",
			},
		},
	)

	if err == nil {
		t.Fatal("runOSProcess() error = nil, want process failure")
	}
	if !strings.Contains(
		err.Error(),
		"-test.run=TestRunOSProcessSilentHelper",
	) {
		t.Fatalf(
			"runOSProcess() error = %v, want process arguments",
			err,
		)
	}
}

func TestRunOSProcessSilentHelper(t *testing.T) {
	if os.Getenv("GO_WANT_SILENT_PROCESS_HELPER") != "1" {
		return
	}

	os.Exit(9)
}
