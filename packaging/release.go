package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

type releaseCreator func(
	context.Context,
	string,
	releaseOptions,
	[]releaseTarget,
	buildRunner,
) ([]string, error)

type releaseDependencies struct {
	create   releaseCreator
	runBuild buildRunner
}

var defaultReleaseTargets = []releaseTarget{
	{
		GOOS:   "linux",
		GOARCH: "amd64",
	},
	{
		GOOS:   "windows",
		GOARCH: "amd64",
	},
}

type releaseTarget struct {
	GOOS   string
	GOARCH string
}

type bundleSpec struct {
	Name        string
	ArchiveName string
	BinaryName  string
	Files       []string
}

func newBundleSpec(
	version string,
	target releaseTarget,
) bundleSpec {
	normalizedVersion := strings.TrimPrefix(
		strings.TrimSpace(version),
		"v",
	)

	name := fmt.Sprintf(
		"telemetry-agent_%s_%s_%s",
		normalizedVersion,
		target.GOOS,
		target.GOARCH,
	)

	binaryName := "agentctl"
	archiveExtension := ".tar.gz"

	if target.GOOS == "windows" {
		binaryName += ".exe"
		archiveExtension = ".zip"
	}

	return bundleSpec{
		Name:        name,
		ArchiveName: name + archiveExtension,
		BinaryName:  binaryName,
		Files: []string{
			binaryName,
			"configs/base/otel.yaml",
			"configs/profiles/laptop.yaml",
			"configs/os/" + target.GOOS + "/otel.yaml",
			"README.md",
		},
	}
}

type releaseOptions struct {
	Version   string
	OutputDir string
	Commit    string
	BuildDate string
}

func (o releaseOptions) Validate() error {
	version := strings.TrimSpace(o.Version)

	if version == "" {
		return fmt.Errorf("release version is required")
	}
	if strings.ContainsAny(version, `\/`) ||
		version == "." ||
		version == ".." {
		return fmt.Errorf(
			"release version %q is unsafe",
			version,
		)
	}

	return nil
}

type buildCommand struct {
	Executable string
	Args       []string
	Env        []string
}

func newGoBuildCommand(
	options releaseOptions,
	target releaseTarget,
	outputPath string,
) buildCommand {
	ldflags := fmt.Sprintf(
		"-s -w -X main.version=%s -X main.commit=%s -X main.buildDate=%s",
		strings.TrimSpace(options.Version),
		strings.TrimSpace(options.Commit),
		strings.TrimSpace(options.BuildDate),
	)

	return buildCommand{
		Executable: "go",
		Args: []string{
			"build",
			"-trimpath",
			"-ldflags",
			ldflags,
			"-o",
			outputPath,
			"./cmd/agentctl",
		},
		Env: []string{
			"CGO_ENABLED=0",
			"GOOS=" + target.GOOS,
			"GOARCH=" + target.GOARCH,
		},
	}
}

func prepareBundle(
	repoRoot string,
	binaryPath string,
	bundleDir string,
	spec bundleSpec,
) error {
	for _, relativePath := range spec.Files {
		sourcePath := filepath.Join(
			repoRoot,
			filepath.FromSlash(relativePath),
		)

		switch relativePath {
		case spec.BinaryName:
			sourcePath = binaryPath

		case "README.md":
			sourcePath = filepath.Join(
				repoRoot,
				"packaging",
				"README.md",
			)
		}

		destinationPath := filepath.Join(
			bundleDir,
			filepath.FromSlash(relativePath),
		)

		if err := copyReleaseFile(sourcePath, destinationPath); err != nil {
			return fmt.Errorf(
				"copy release file %q: %w",
				relativePath,
				err,
			)
		}
	}

	return nil
}

func copyReleaseFile(
	sourcePath string,
	destinationPath string,
) error {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("inspect source file: %w", err)
	}

	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("read source file: %w", err)
	}

	if err := os.MkdirAll(
		filepath.Dir(destinationPath),
		0o755,
	); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}

	if err := os.WriteFile(
		destinationPath,
		content,
		info.Mode().Perm(),
	); err != nil {
		return fmt.Errorf("write destination file: %w", err)
	}

	return nil
}

func writeTarGzArchive(
	bundleDir string,
	archivePath string,
) error {
	temporaryFile, err := os.CreateTemp(
		filepath.Dir(archivePath),
		"."+filepath.Base(archivePath)+"-",
	)
	if err != nil {
		return fmt.Errorf("create temporary tar.gz archive: %w", err)
	}

	temporaryPath := temporaryFile.Name()
	defer os.Remove(temporaryPath)

	gzipWriter := gzip.NewWriter(temporaryFile)
	tarWriter := tar.NewWriter(gzipWriter)

	parentDir := filepath.Dir(bundleDir)

	walkErr := filepath.Walk(
		bundleDir,
		func(
			filePath string,
			info os.FileInfo,
			walkErr error,
		) error {
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() {
				return nil
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf(
					"release bundle entry %q is not a regular file",
					filePath,
				)
			}

			relativePath, err := filepath.Rel(parentDir, filePath)
			if err != nil {
				return fmt.Errorf("get archive entry path: %w", err)
			}

			header, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return fmt.Errorf("create tar header: %w", err)
			}
			header.Name = path.Clean(
				filepath.ToSlash(relativePath),
			)

			if err := tarWriter.WriteHeader(header); err != nil {
				return fmt.Errorf("write tar header: %w", err)
			}

			sourceFile, err := os.Open(filePath)
			if err != nil {
				return fmt.Errorf("open release bundle file: %w", err)
			}

			_, copyErr := io.Copy(tarWriter, sourceFile)
			closeErr := sourceFile.Close()

			if copyErr != nil {
				return fmt.Errorf("write tar file content: %w", copyErr)
			}
			if closeErr != nil {
				return fmt.Errorf("close release bundle file: %w", closeErr)
			}

			return nil
		},
	)

	tarCloseErr := tarWriter.Close()
	gzipCloseErr := gzipWriter.Close()
	fileCloseErr := temporaryFile.Close()

	if walkErr != nil {
		return fmt.Errorf("archive release bundle: %w", walkErr)
	}
	if tarCloseErr != nil {
		return fmt.Errorf("close tar archive: %w", tarCloseErr)
	}
	if gzipCloseErr != nil {
		return fmt.Errorf("close gzip archive: %w", gzipCloseErr)
	}
	if fileCloseErr != nil {
		return fmt.Errorf("close archive file: %w", fileCloseErr)
	}

	if err := os.Rename(temporaryPath, archivePath); err != nil {
		return fmt.Errorf("publish tar.gz archive: %w", err)
	}

	return nil
}

func writeZipArchive(
	bundleDir string,
	archivePath string,
) error {
	temporaryFile, err := os.CreateTemp(
		filepath.Dir(archivePath),
		"."+filepath.Base(archivePath)+"-",
	)
	if err != nil {
		return fmt.Errorf("create temporary zip archive: %w", err)
	}

	temporaryPath := temporaryFile.Name()
	defer os.Remove(temporaryPath)

	zipWriter := zip.NewWriter(temporaryFile)
	parentDir := filepath.Dir(bundleDir)

	walkErr := filepath.Walk(
		bundleDir,
		func(
			filePath string,
			info os.FileInfo,
			walkErr error,
		) error {
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() {
				return nil
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf(
					"release bundle entry %q is not a regular file",
					filePath,
				)
			}

			relativePath, err := filepath.Rel(parentDir, filePath)
			if err != nil {
				return fmt.Errorf("get archive entry path: %w", err)
			}

			header, err := zip.FileInfoHeader(info)
			if err != nil {
				return fmt.Errorf("create zip header: %w", err)
			}
			header.Name = path.Clean(
				filepath.ToSlash(relativePath),
			)
			header.Method = zip.Deflate

			destinationFile, err := zipWriter.CreateHeader(header)
			if err != nil {
				return fmt.Errorf("create zip entry: %w", err)
			}

			sourceFile, err := os.Open(filePath)
			if err != nil {
				return fmt.Errorf("open release bundle file: %w", err)
			}

			_, copyErr := io.Copy(destinationFile, sourceFile)
			closeErr := sourceFile.Close()

			if copyErr != nil {
				return fmt.Errorf("write zip file content: %w", copyErr)
			}
			if closeErr != nil {
				return fmt.Errorf("close release bundle file: %w", closeErr)
			}

			return nil
		},
	)

	zipCloseErr := zipWriter.Close()
	fileCloseErr := temporaryFile.Close()

	if walkErr != nil {
		return fmt.Errorf("archive release bundle: %w", walkErr)
	}
	if zipCloseErr != nil {
		return fmt.Errorf("close zip archive: %w", zipCloseErr)
	}
	if fileCloseErr != nil {
		return fmt.Errorf("close archive file: %w", fileCloseErr)
	}

	if err := os.Rename(temporaryPath, archivePath); err != nil {
		return fmt.Errorf("publish zip archive: %w", err)
	}

	return nil
}

func writeChecksums(
	manifestPath string,
	archivePaths []string,
) error {
	type archive struct {
		path string
		name string
	}

	archives := make([]archive, 0, len(archivePaths))

	for _, archivePath := range archivePaths {
		archives = append(archives, archive{
			path: archivePath,
			name: filepath.Base(archivePath),
		})
	}

	sort.Slice(archives, func(left, right int) bool {
		return archives[left].name < archives[right].name
	})

	var manifest strings.Builder

	for _, archive := range archives {
		file, err := os.Open(archive.path)
		if err != nil {
			return fmt.Errorf(
				"open release archive %q: %w",
				archive.name,
				err,
			)
		}

		hasher := sha256.New()
		_, copyErr := io.Copy(hasher, file)
		closeErr := file.Close()

		if copyErr != nil {
			return fmt.Errorf(
				"hash release archive %q: %w",
				archive.name,
				copyErr,
			)
		}
		if closeErr != nil {
			return fmt.Errorf(
				"close release archive %q: %w",
				archive.name,
				closeErr,
			)
		}

		fmt.Fprintf(
			&manifest,
			"%x  %s\n",
			hasher.Sum(nil),
			archive.name,
		)
	}

	if err := os.WriteFile(
		manifestPath,
		[]byte(manifest.String()),
		0o644,
	); err != nil {
		return fmt.Errorf("write release checksums: %w", err)
	}

	return nil
}

type buildRunner func(context.Context, buildCommand) error

func createRelease(
	ctx context.Context,
	repoRoot string,
	options releaseOptions,
	targets []releaseTarget,
	runBuild buildRunner,
) ([]string, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(repoRoot) == "" {
		return nil, fmt.Errorf("repository root is required")
	}
	if strings.TrimSpace(options.OutputDir) == "" {
		return nil, fmt.Errorf("release output directory is required")
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("at least one release target is required")
	}
	if runBuild == nil {
		return nil, fmt.Errorf("release build runner is required")
	}

	outputDir := strings.TrimSpace(options.OutputDir)

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create release output directory: %w", err)
	}

	stagingDir, err := os.MkdirTemp(
		outputDir,
		".telemetry-agent-release-",
	)
	if err != nil {
		return nil, fmt.Errorf("create release staging directory: %w", err)
	}
	defer os.RemoveAll(stagingDir)

	archivePaths := make([]string, 0, len(targets))

	for _, target := range targets {
		spec := newBundleSpec(options.Version, target)

		targetStagingDir := filepath.Join(stagingDir, spec.Name)
		if err := os.MkdirAll(targetStagingDir, 0o755); err != nil {
			return nil, fmt.Errorf(
				"create %s staging directory: %w",
				spec.Name,
				err,
			)
		}

		binaryPath := filepath.Join(
			targetStagingDir,
			spec.BinaryName,
		)

		if err := runBuild(
			ctx,
			newGoBuildCommand(options, target, binaryPath),
		); err != nil {
			return nil, fmt.Errorf(
				"build agentctl for %s/%s: %w",
				target.GOOS,
				target.GOARCH,
				err,
			)
		}

		bundleDir := filepath.Join(targetStagingDir, spec.Name)

		if err := prepareBundle(
			repoRoot,
			binaryPath,
			bundleDir,
			spec,
		); err != nil {
			return nil, fmt.Errorf(
				"prepare %s release bundle: %w",
				spec.Name,
				err,
			)
		}

		archivePath := filepath.Join(outputDir, spec.ArchiveName)

		switch target.GOOS {
		case "linux":
			err = writeTarGzArchive(bundleDir, archivePath)

		case "windows":
			err = writeZipArchive(bundleDir, archivePath)

		default:
			return nil, fmt.Errorf(
				"unsupported release operating system %q",
				target.GOOS,
			)
		}

		if err != nil {
			return nil, fmt.Errorf(
				"create %s release archive: %w",
				spec.Name,
				err,
			)
		}

		archivePaths = append(archivePaths, archivePath)
	}

	if err := writeChecksums(
		filepath.Join(outputDir, "checksums.txt"),
		archivePaths,
	); err != nil {
		return nil, err
	}

	return archivePaths, nil
}

func runOSBuild(
	ctx context.Context,
	command buildCommand,
) error {
	if strings.TrimSpace(command.Executable) == "" {
		return fmt.Errorf("release build executable is required")
	}

	process := exec.CommandContext(
		ctx,
		command.Executable,
		command.Args...,
	)
	process.Env = append(os.Environ(), command.Env...)

	output, err := process.CombinedOutput()
	if err == nil {
		return nil
	}

	diagnostics := strings.TrimSpace(string(output))
	if diagnostics == "" {
		return fmt.Errorf(
			"run release build command %q: %w",
			command.Executable,
			err,
		)
	}

	return fmt.Errorf(
		"run release build command %q with arguments %#v: %w: %s",
		command.Executable,
		command.Args,
		err,
		diagnostics,
	)
}

func parseReleaseOptions(
	args []string,
	stderr io.Writer,
) (releaseOptions, error) {
	flags := flag.NewFlagSet(
		"release",
		flag.ContinueOnError,
	)
	flags.SetOutput(stderr)

	version := flags.String(
		"version",
		"",
		"release version, for example v0.1.0",
	)
	outputDir := flags.String(
		"out",
		"dist",
		"output directory for release archives",
	)
	commit := flags.String(
		"commit",
		"none",
		"source commit included in agentctl version output",
	)
	buildDate := flags.String(
		"build-date",
		"unknown",
		"RFC3339 build timestamp included in agentctl version output",
	)

	if err := flags.Parse(args); err != nil {
		return releaseOptions{}, err
	}
	if flags.NArg() != 0 {
		return releaseOptions{}, fmt.Errorf(
			"release does not accept positional arguments",
		)
	}

	options := releaseOptions{
		Version:   strings.TrimSpace(*version),
		OutputDir: strings.TrimSpace(*outputDir),
		Commit:    strings.TrimSpace(*commit),
		BuildDate: strings.TrimSpace(*buildDate),
	}

	if err := options.Validate(); err != nil {
		return releaseOptions{}, err
	}

	return options, nil
}

func runRelease(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	repoRoot string,
	dependencies releaseDependencies,
) int {
	options, err := parseReleaseOptions(args, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "release options: %v\n", err)
		return 2
	}

	create := dependencies.create
	if create == nil {
		create = createRelease
	}

	runBuild := dependencies.runBuild
	if runBuild == nil {
		runBuild = runOSBuild
	}

	archivePaths, err := create(
		ctx,
		repoRoot,
		options,
		defaultReleaseTargets,
		runBuild,
	)
	if err != nil {
		fmt.Fprintf(stderr, "create release: %v\n", err)
		return 1
	}

	fmt.Fprintln(stdout, "Created release archives:")
	for _, archivePath := range archivePaths {
		fmt.Fprintf(stdout, "- %s\n", archivePath)
	}
	fmt.Fprintf(
		stdout,
		"Checksums: %s\n",
		filepath.Join(options.OutputDir, "checksums.txt"),
	)

	return 0
}

func runReleaseFromWorkingDirectory(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	dependencies releaseDependencies,
	getWorkingDirectory func() (string, error),
) int {
	if getWorkingDirectory == nil {
		fmt.Fprintln(stderr, "determine repository root: working directory lookup is required")
		return 1
	}

	repoRoot, err := getWorkingDirectory()
	if err != nil {
		fmt.Fprintf(stderr, "determine repository root: %v\n", err)
		return 1
	}

	return runRelease(
		ctx,
		args,
		stdout,
		stderr,
		repoRoot,
		dependencies,
	)
}

func main() {
	exitCode := runReleaseFromWorkingDirectory(
		context.Background(),
		os.Args[1:],
		os.Stdout,
		os.Stderr,
		releaseDependencies{},
		os.Getwd,
	)

	os.Exit(exitCode)
}
