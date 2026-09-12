package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/config"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

const (
	defaultValidationEndpoint = "127.0.0.1:4317"
	defaultValidationTimeout  = 2 * time.Minute
)

type dependencies struct {
	collectPlatform func() (identity.PlatformInfo, error)
	runBootstrap    bootstrapRunFunc
	downloader      bootstrap.Downloader
	runner          bootstrap.CommandRunner
}

type bootstrapRunFunc func(
	context.Context,
	bootstrap.Options,
	bootstrap.Downloader,
	bootstrap.CommandRunner,
) (bootstrap.Result, error)

func main() {
	deps := dependencies{
		collectPlatform: identity.CollectPlatformInfo,
		runBootstrap:    bootstrap.Run,
		downloader:      bootstrap.HTTPDownloader{},
		runner:          bootstrap.OSCommandRunner{},
	}

	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, deps))
}

func run(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	deps dependencies,
) int {
	if len(args) == 0 || args[0] != "bootstrap" {
		fmt.Fprintln(stderr, "usage: agentctl bootstrap --config-root <path> --install-dir <path> --config-path <path> [--validation-endpoint <host:port>] [--timeout <duration>]")
		return 2
	}

	return runBootstrap(ctx, args[1:], stdout, stderr, deps)
}

func runBootstrap(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	deps dependencies,
) int {
	flags := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: agentctl bootstrap --config-root <path> --install-dir <path> --config-path <path> [--validation-endpoint <host:port>] [--timeout <duration>]")
	}

	configRoot := flags.String("config-root", "", "root directory containing configuration fragments")
	installDir := flags.String("install-dir", "", "Collector installation root")
	configPath := flags.String("config-path", "", "final rendered Collector configuration path")
	validationEndpoint := flags.String("validation-endpoint", defaultValidationEndpoint, "endpoint supplied only to Collector validation")
	validationTimeout := flags.Duration("timeout", defaultValidationTimeout, "Collector validation timeout")

	if err := flags.Parse(args); err != nil {
		flags.Usage()
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "usage error: bootstrap does not accept positional arguments")
		flags.Usage()
		return 2
	}
	if *configRoot == "" {
		return usageError(stderr, flags, "--config-root is required")
	}
	if *installDir == "" {
		return usageError(stderr, flags, "--install-dir is required")
	}
	if *configPath == "" {
		return usageError(stderr, flags, "--config-path is required")
	}

	collectPlatform := deps.collectPlatform
	if collectPlatform == nil {
		collectPlatform = identity.CollectPlatformInfo
	}
	platform, err := collectPlatform()
	if err != nil {
		fmt.Fprintf(stderr, "collect platform information: %v\n", err)
		return 1
	}

	layers, err := configurationLayers(*configRoot, platform.OS)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	runBootstrap := deps.runBootstrap
	if runBootstrap == nil {
		runBootstrap = bootstrap.Run
	}
	result, err := runBootstrap(ctx, bootstrap.Options{
		Platform:   platform,
		InstallDir: *installDir,
		ConfigPath: *configPath,
		ConfigInput: config.RenderInput{
			Platform: platform,
			Layers:   layers,
		},
		ValidationEnvironment: []string{
			"OTEL_GATEWAY_ENDPOINT=" + *validationEndpoint,
		},
		ValidationTimeout: *validationTimeout,
	}, deps.downloader, deps.runner)
	if err != nil {
		fmt.Fprintf(stderr, "bootstrap Collector: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Collector version: %s\n", result.Artifact.Version)
	fmt.Fprintf(stdout, "Collector binary path: %s\n", result.BinaryPath)
	fmt.Fprintf(stdout, "Final configuration path: %s\n", result.ConfigPath)
	fmt.Fprintf(stdout, "Collector installation reused: %t\n", result.Reused)

	return 0
}

func usageError(stderr io.Writer, flags *flag.FlagSet, message string) int {
	fmt.Fprintln(stderr, "usage error: "+message)
	flags.Usage()
	return 2
}

func configurationLayers(configRoot string, osName string) ([]config.Layer, error) {
	if osName != "linux" && osName != "windows" {
		return nil, fmt.Errorf("unsupported operating system %q", osName)
	}

	return []config.Layer{
		{Name: "base", Path: filepath.Join(configRoot, "base", "otel.yaml")},
		{Name: "profile", Path: filepath.Join(configRoot, "profiles", "laptop.yaml")},
		{Name: "os", Path: filepath.Join(configRoot, "os", osName, "otel.yaml")},
	}, nil
}
