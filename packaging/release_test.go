package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNewBundleSpecUsesExpectedLinuxLayout(t *testing.T) {
	spec := newBundleSpec(
		"v0.1.0",
		releaseTarget{
			GOOS:   "linux",
			GOARCH: "amd64",
		},
	)

	if spec.Name != "telemetry-agent_0.1.0_linux_amd64" {
		t.Fatalf(
			"bundle name = %q, want %q",
			spec.Name,
			"telemetry-agent_0.1.0_linux_amd64",
		)
	}

	if spec.ArchiveName != "telemetry-agent_0.1.0_linux_amd64.tar.gz" {
		t.Fatalf(
			"archive name = %q, want Linux tar.gz",
			spec.ArchiveName,
		)
	}

	if spec.BinaryName != "agentctl" {
		t.Fatalf(
			"binary name = %q, want agentctl",
			spec.BinaryName,
		)
	}

	wantFiles := []string{
		"agentctl",
		"configs/base/otel.yaml",
		"configs/profiles/laptop.yaml",
		"configs/os/linux/otel.yaml",
		"README.md",
	}

	if !reflect.DeepEqual(spec.Files, wantFiles) {
		t.Fatalf(
			"bundle files = %#v, want %#v",
			spec.Files,
			wantFiles,
		)
	}
}

func TestNewBundleSpecUsesExpectedWindowsLayout(t *testing.T) {
	spec := newBundleSpec(
		"v0.1.0",
		releaseTarget{
			GOOS:   "windows",
			GOARCH: "amd64",
		},
	)

	if spec.Name != "telemetry-agent_0.1.0_windows_amd64" {
		t.Fatalf(
			"bundle name = %q, want Windows bundle name",
			spec.Name,
		)
	}

	if spec.ArchiveName != "telemetry-agent_0.1.0_windows_amd64.zip" {
		t.Fatalf(
			"archive name = %q, want Windows zip",
			spec.ArchiveName,
		)
	}

	if spec.BinaryName != "agentctl.exe" {
		t.Fatalf(
			"binary name = %q, want agentctl.exe",
			spec.BinaryName,
		)
	}

	wantFiles := []string{
		"agentctl.exe",
		"configs/base/otel.yaml",
		"configs/profiles/laptop.yaml",
		"configs/os/windows/otel.yaml",
		"README.md",
	}

	if !reflect.DeepEqual(spec.Files, wantFiles) {
		t.Fatalf(
			"bundle files = %#v, want %#v",
			spec.Files,
			wantFiles,
		)
	}
}

func TestReleaseOptionsValidateRejectsMissingVersion(t *testing.T) {
	options := releaseOptions{
		OutputDir: "dist",
	}

	err := options.Validate()

	if err == nil {
		t.Fatal("releaseOptions.Validate() error = nil, want an error")
	}
	if !strings.Contains(err.Error(), "release version") {
		t.Fatalf(
			"releaseOptions.Validate() error = %q, want version error",
			err,
		)
	}
}

func TestReleaseOptionsValidateRejectsUnsafeVersion(t *testing.T) {
	for _, version := range []string{
		"../v0.1.0",
		`v0.1.0\windows`,
		"v0.1.0/linux",
	} {
		t.Run(version, func(t *testing.T) {
			err := (releaseOptions{
				Version:   version,
				OutputDir: "dist",
			}).Validate()

			if err == nil {
				t.Fatal("releaseOptions.Validate() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), "unsafe") {
				t.Fatalf(
					"releaseOptions.Validate() error = %q, want unsafe-version error",
					err,
				)
			}
		})
	}
}

func TestNewGoBuildCommandBuildsLinuxAgentctl(t *testing.T) {
	command := newGoBuildCommand(
		releaseOptions{
			Version:   "v0.1.0",
			Commit:    "abc1234",
			BuildDate: "2026-09-18T00:00:00Z",
		},
		releaseTarget{
			GOOS:   "linux",
			GOARCH: "amd64",
		},
		"/temporary/bundle/agentctl",
	)

	want := buildCommand{
		Executable: "go",
		Args: []string{
			"build",
			"-trimpath",
			"-ldflags",
			"-s -w " +
				"-X main.version=v0.1.0 " +
				"-X main.commit=abc1234 " +
				"-X main.buildDate=2026-09-18T00:00:00Z",
			"-o",
			"/temporary/bundle/agentctl",
			"./cmd/agentctl",
		},
		Env: []string{
			"CGO_ENABLED=0",
			"GOOS=linux",
			"GOARCH=amd64",
		},
	}

	if !reflect.DeepEqual(command, want) {
		t.Fatalf(
			"build command = %#v, want %#v",
			command,
			want,
		)
	}
}

func TestNewGoBuildCommandBuildsWindowsAgentctl(t *testing.T) {
	command := newGoBuildCommand(
		releaseOptions{
			Version:   "v0.1.0",
			Commit:    "abc1234",
			BuildDate: "2026-09-18T00:00:00Z",
		},
		releaseTarget{
			GOOS:   "windows",
			GOARCH: "amd64",
		},
		"/temporary/bundle/agentctl.exe",
	)

	if !reflect.DeepEqual(command.Env, []string{
		"CGO_ENABLED=0",
		"GOOS=windows",
		"GOARCH=amd64",
	}) {
		t.Fatalf("build environment = %#v, want Windows target", command.Env)
	}

	if command.Args[len(command.Args)-2] != "/temporary/bundle/agentctl.exe" {
		t.Fatalf(
			"build output = %q, want Windows executable path",
			command.Args[len(command.Args)-2],
		)
	}
}

func TestPrepareBundleCopiesExpectedFiles(t *testing.T) {
	repoRoot := t.TempDir()

	binaryPath := filepath.Join(repoRoot, "build", "agentctl")
	writeReleaseFixtureFile(t, binaryPath, "agentctl binary", 0o755)

	writeReleaseFixtureFile(
		t,
		filepath.Join(repoRoot, "configs", "base", "otel.yaml"),
		"base config",
		0o644,
	)
	writeReleaseFixtureFile(
		t,
		filepath.Join(repoRoot, "configs", "profiles", "laptop.yaml"),
		"profile config",
		0o644,
	)
	writeReleaseFixtureFile(
		t,
		filepath.Join(repoRoot, "configs", "os", "linux", "otel.yaml"),
		"linux config",
		0o644,
	)
	writeReleaseFixtureFile(
		t,
		filepath.Join(repoRoot, "packaging", "README.md"),
		"release instructions",
		0o644,
	)

	spec := newBundleSpec(
		"v0.1.0",
		releaseTarget{
			GOOS:   "linux",
			GOARCH: "amd64",
		},
	)
	bundleDir := filepath.Join(t.TempDir(), spec.Name)

	if err := prepareBundle(repoRoot, binaryPath, bundleDir, spec); err != nil {
		t.Fatalf("prepareBundle() error = %v", err)
	}

	wantFiles := map[string]string{
		"agentctl":                     "agentctl binary",
		"configs/base/otel.yaml":       "base config",
		"configs/profiles/laptop.yaml": "profile config",
		"configs/os/linux/otel.yaml":   "linux config",
		"README.md":                    "release instructions",
	}

	for relativePath, wantContent := range wantFiles {
		content, err := os.ReadFile(
			filepath.Join(bundleDir, relativePath),
		)
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", relativePath, err)
		}
		if string(content) != wantContent {
			t.Fatalf(
				"file %q content = %q, want %q",
				relativePath,
				content,
				wantContent,
			)
		}
	}

	info, err := os.Stat(filepath.Join(bundleDir, "agentctl"))
	if err != nil {
		t.Fatalf("Stat(agentctl) error = %v", err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Fatalf(
			"agentctl mode = %04o, want owner-executable",
			info.Mode().Perm(),
		)
	}
}

func writeReleaseFixtureFile(
	t *testing.T,
	path string,
	content string,
	mode os.FileMode,
) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func TestWriteTarGzArchiveContainsBundleFiles(t *testing.T) {
	bundleName := "telemetry-agent_0.1.0_linux_amd64"
	bundleDir := filepath.Join(t.TempDir(), bundleName)

	writeReleaseFixtureFile(
		t,
		filepath.Join(bundleDir, "agentctl"),
		"binary",
		0o755,
	)
	writeReleaseFixtureFile(
		t,
		filepath.Join(bundleDir, "configs", "base", "otel.yaml"),
		"base config",
		0o644,
	)
	writeReleaseFixtureFile(
		t,
		filepath.Join(bundleDir, "README.md"),
		"instructions",
		0o644,
	)

	archivePath := filepath.Join(
		t.TempDir(),
		bundleName+".tar.gz",
	)

	if err := writeTarGzArchive(bundleDir, archivePath); err != nil {
		t.Fatalf("writeTarGzArchive() error = %v", err)
	}

	gotFiles := readTarGzFileNames(t, archivePath)

	for _, want := range []string{
		bundleName + "/agentctl",
		bundleName + "/configs/base/otel.yaml",
		bundleName + "/README.md",
	} {
		if !gotFiles[want] {
			t.Fatalf(
				"tar.gz does not contain %q; files = %#v",
				want,
				gotFiles,
			)
		}
	}
}

func readTarGzFileNames(
	t *testing.T,
	archivePath string,
) map[string]bool {
	t.Helper()

	file, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("gzip.NewReader() error = %v", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	files := make(map[string]bool)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			return files
		}
		if err != nil {
			t.Fatalf("tarReader.Next() error = %v", err)
		}

		files[header.Name] = true
	}
}

func TestWriteZipArchiveContainsBundleFiles(t *testing.T) {
	bundleName := "telemetry-agent_0.1.0_windows_amd64"
	bundleDir := filepath.Join(t.TempDir(), bundleName)

	writeReleaseFixtureFile(
		t,
		filepath.Join(bundleDir, "agentctl.exe"),
		"windows binary",
		0o755,
	)
	writeReleaseFixtureFile(
		t,
		filepath.Join(bundleDir, "configs", "base", "otel.yaml"),
		"base config",
		0o644,
	)
	writeReleaseFixtureFile(
		t,
		filepath.Join(bundleDir, "README.md"),
		"instructions",
		0o644,
	)

	archivePath := filepath.Join(
		t.TempDir(),
		bundleName+".zip",
	)

	if err := writeZipArchive(bundleDir, archivePath); err != nil {
		t.Fatalf("writeZipArchive() error = %v", err)
	}

	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatalf("zip.OpenReader() error = %v", err)
	}
	defer archive.Close()

	files := make(map[string]bool)
	for _, file := range archive.File {
		files[file.Name] = true
	}

	for _, want := range []string{
		bundleName + "/agentctl.exe",
		bundleName + "/configs/base/otel.yaml",
		bundleName + "/README.md",
	} {
		if !files[want] {
			t.Fatalf(
				"zip does not contain %q; files = %#v",
				want,
				files,
			)
		}
	}
}

func TestWriteChecksumsWritesSortedArchiveDigests(t *testing.T) {
	outputDir := t.TempDir()

	windowsArchive := filepath.Join(
		outputDir,
		"telemetry-agent_0.1.0_windows_amd64.zip",
	)
	linuxArchive := filepath.Join(
		outputDir,
		"telemetry-agent_0.1.0_linux_amd64.tar.gz",
	)

	writeReleaseFixtureFile(
		t,
		windowsArchive,
		"windows archive",
		0o644,
	)
	writeReleaseFixtureFile(
		t,
		linuxArchive,
		"linux archive",
		0o644,
	)

	manifestPath := filepath.Join(outputDir, "checksums.txt")

	if err := writeChecksums(
		manifestPath,
		[]string{windowsArchive, linuxArchive},
	); err != nil {
		t.Fatalf("writeChecksums() error = %v", err)
	}

	linuxDigest := sha256.Sum256([]byte("linux archive"))
	windowsDigest := sha256.Sum256([]byte("windows archive"))

	want := fmt.Sprintf(
		"%x  telemetry-agent_0.1.0_linux_amd64.tar.gz\n"+
			"%x  telemetry-agent_0.1.0_windows_amd64.zip\n",
		linuxDigest,
		windowsDigest,
	)

	content, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile(checksums.txt) error = %v", err)
	}

	if string(content) != want {
		t.Fatalf(
			"checksums content = %q, want %q",
			content,
			want,
		)
	}
}

func TestCreateReleaseBuildsLinuxArchiveAndChecksums(t *testing.T) {
	repoRoot := t.TempDir()
	outputDir := filepath.Join(t.TempDir(), "dist")

	writeReleaseFixtureFile(
		t,
		filepath.Join(repoRoot, "configs", "base", "otel.yaml"),
		"base config",
		0o644,
	)
	writeReleaseFixtureFile(
		t,
		filepath.Join(repoRoot, "configs", "profiles", "laptop.yaml"),
		"profile config",
		0o644,
	)
	writeReleaseFixtureFile(
		t,
		filepath.Join(repoRoot, "configs", "os", "linux", "otel.yaml"),
		"linux config",
		0o644,
	)
	writeReleaseFixtureFile(
		t,
		filepath.Join(repoRoot, "configs", "os", "windows", "otel.yaml"),
		"windows config",
		0o644,
	)
	writeReleaseFixtureFile(
		t,
		filepath.Join(repoRoot, "packaging", "README.md"),
		"release instructions",
		0o644,
	)

	var buildCalls []buildCommand

	archivePaths, err := createRelease(
		context.Background(),
		repoRoot,
		releaseOptions{
			Version:   "v0.1.0",
			OutputDir: outputDir,
			Commit:    "abc1234",
			BuildDate: "2026-09-18T00:00:00Z",
		},
		[]releaseTarget{
			{
				GOOS:   "linux",
				GOARCH: "amd64",
			},
			{
				GOOS:   "windows",
				GOARCH: "amd64",
			},
		},
		func(
			ctx context.Context,
			command buildCommand,
		) error {
			buildCalls = append(buildCalls, command)

			outputPath := buildCommandOutputPath(t, command)
			writeReleaseFixtureFile(
				t,
				outputPath,
				"built agentctl",
				0o755,
			)

			return nil
		},
	)
	if err != nil {
		t.Fatalf("createRelease() error = %v", err)
	}

	wantArchives := []string{
		filepath.Join(
			outputDir,
			"telemetry-agent_0.1.0_linux_amd64.tar.gz",
		),
		filepath.Join(
			outputDir,
			"telemetry-agent_0.1.0_windows_amd64.zip",
		),
	}

	if !reflect.DeepEqual(archivePaths, wantArchives) {
		t.Fatalf(
			"archive paths = %#v, want %#v",
			archivePaths,
			wantArchives,
		)
	}
	if len(buildCalls) != 2 {
		t.Fatalf("build calls = %d, want 2", len(buildCalls))
	}

	for _, archivePath := range wantArchives {
		if _, err := os.Stat(archivePath); err != nil {
			t.Fatalf("Stat(%q) error = %v", archivePath, err)
		}
	}

	checksums, err := os.ReadFile(
		filepath.Join(outputDir, "checksums.txt"),
	)
	if err != nil {
		t.Fatalf("ReadFile(checksums.txt) error = %v", err)
	}
	if !strings.Contains(
		string(checksums),
		"telemetry-agent_0.1.0_linux_amd64.tar.gz",
	) {
		t.Fatalf(
			"checksums = %q, want Linux archive entry",
			checksums,
		)
	}

	if !strings.Contains(
		string(checksums),
		"telemetry-agent_0.1.0_windows_amd64.zip",
	) {
		t.Fatalf(
			"checksums = %q, want Windows archive entry",
			checksums,
		)
	}
}

func buildCommandOutputPath(
	t *testing.T,
	command buildCommand,
) string {
	t.Helper()

	for index, argument := range command.Args {
		if argument == "-o" && index+1 < len(command.Args) {
			return command.Args[index+1]
		}
	}

	t.Fatalf("build command has no output path: %#v", command)
	return ""
}

func TestRunOSBuildRunsCommandWithEnvironment(t *testing.T) {
	err := runOSBuild(
		context.Background(),
		buildCommand{
			Executable: os.Args[0],
			Args: []string{
				"-test.run=TestRunOSBuildHelper",
				"--",
			},
			Env: []string{
				"AR_IMMS_RELEASE_BUILD_HELPER=1",
			},
		},
	)
	if err != nil {
		t.Fatalf("runOSBuild() error = %v", err)
	}
}

func TestRunOSBuildHelper(t *testing.T) {
	if os.Getenv("AR_IMMS_RELEASE_BUILD_HELPER") == "" {
		return
	}

	if os.Getenv("AR_IMMS_RELEASE_BUILD_HELPER") != "1" {
		os.Exit(7)
	}

	os.Exit(0)
}

func TestRunOSBuildPreservesFailureDiagnostics(t *testing.T) {
	err := runOSBuild(
		context.Background(),
		buildCommand{
			Executable: os.Args[0],
			Args: []string{
				"-test.run=TestRunOSBuildFailureHelper",
				"--",
			},
			Env: []string{
				"AR_IMMS_RELEASE_BUILD_FAILURE_HELPER=1",
			},
		},
	)

	if err == nil {
		t.Fatal("runOSBuild() error = nil, want failure")
	}
	if !strings.Contains(
		err.Error(),
		"intentional release build failure",
	) {
		t.Fatalf(
			"runOSBuild() error = %q, want diagnostics",
			err,
		)
	}
}

func TestRunOSBuildFailureHelper(t *testing.T) {
	if os.Getenv("AR_IMMS_RELEASE_BUILD_FAILURE_HELPER") == "" {
		return
	}

	fmt.Fprint(os.Stderr, "intentional release build failure")
	os.Exit(7)
}

func TestParseReleaseOptionsUsesVersionAndDefaultOutput(
	t *testing.T,
) {
	var stderr bytes.Buffer

	options, err := parseReleaseOptions(
		[]string{"-version", "v0.1.0"},
		&stderr,
	)
	if err != nil {
		t.Fatalf("parseReleaseOptions() error = %v", err)
	}

	if options.Version != "v0.1.0" {
		t.Fatalf(
			"release version = %q, want v0.1.0",
			options.Version,
		)
	}
	if options.OutputDir != "dist" {
		t.Fatalf(
			"release output directory = %q, want dist",
			options.OutputDir,
		)
	}
	if stderr.Len() != 0 {
		t.Fatalf(
			"parse stderr = %q, want empty",
			stderr.String(),
		)
	}
}

func TestRunReleaseCreatesPlatformArchives(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	var gotRepoRoot string
	var gotOptions releaseOptions
	var gotTargets []releaseTarget

	exitCode := runRelease(
		context.Background(),
		[]string{
			"-version",
			"v0.1.0",
			"-out",
			"dist",
			"-commit",
			"abc1234",
			"-build-date",
			"2026-09-18T00:00:00Z",
		},
		&stdout,
		&stderr,
		"/repository",
		releaseDependencies{
			create: func(
				ctx context.Context,
				repoRoot string,
				options releaseOptions,
				targets []releaseTarget,
				runBuild buildRunner,
			) ([]string, error) {
				gotRepoRoot = repoRoot
				gotOptions = options
				gotTargets = targets

				return []string{
					"dist/telemetry-agent_0.1.0_linux_amd64.tar.gz",
					"dist/telemetry-agent_0.1.0_windows_amd64.zip",
				}, nil
			},
		},
	)

	if exitCode != 0 {
		t.Fatalf("runRelease() exit code = %d, want 0", exitCode)
	}
	if gotRepoRoot != "/repository" {
		t.Fatalf("repository root = %q, want /repository", gotRepoRoot)
	}
	if gotOptions.Version != "v0.1.0" {
		t.Fatalf("release options = %+v, want requested version", gotOptions)
	}

	wantTargets := []releaseTarget{
		{GOOS: "linux", GOARCH: "amd64"},
		{GOOS: "windows", GOARCH: "amd64"},
	}
	if !reflect.DeepEqual(gotTargets, wantTargets) {
		t.Fatalf("targets = %#v, want %#v", gotTargets, wantTargets)
	}

	wantOutput := "" +
		"Created release archives:\n" +
		"- dist/telemetry-agent_0.1.0_linux_amd64.tar.gz\n" +
		"- dist/telemetry-agent_0.1.0_windows_amd64.zip\n" +
		"Checksums: dist/checksums.txt\n"

	if stdout.String() != wantOutput {
		t.Fatalf(
			"release stdout = %q, want %q",
			stdout.String(),
			wantOutput,
		)
	}
	if stderr.Len() != 0 {
		t.Fatalf("release stderr = %q, want empty", stderr.String())
	}
}

func TestRunReleaseFromWorkingDirectoryUsesRepositoryRoot(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	var gotRepoRoot string

	exitCode := runReleaseFromWorkingDirectory(
		context.Background(),
		[]string{"-version", "v0.1.0"},
		&stdout,
		&stderr,
		releaseDependencies{
			create: func(
				ctx context.Context,
				repoRoot string,
				options releaseOptions,
				targets []releaseTarget,
				runBuild buildRunner,
			) ([]string, error) {
				gotRepoRoot = repoRoot
				return []string{
					"dist/telemetry-agent_0.1.0_linux_amd64.tar.gz",
				}, nil
			},
		},
		func() (string, error) {
			return "/repository", nil
		},
	)

	if exitCode != 0 {
		t.Fatalf(
			"runReleaseFromWorkingDirectory() exit code = %d, want 0",
			exitCode,
		)
	}
	if gotRepoRoot != "/repository" {
		t.Fatalf("repository root = %q, want /repository", gotRepoRoot)
	}
}
