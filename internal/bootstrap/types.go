package bootstrap

import (
	"context"
	"io"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/identity"
)

const (
	collectorVersion = "0.160.0"
	maxArtifactBytes = 256 << 20
	maxCommandOutput = 64 << 10
)

type Artifact struct {
	Version      string
	OS           string
	Architecture string
	URL          string
	SHA256       string
	ArchiveName  string
	BinaryName   string
}

type Options struct {
	Platform          identity.PlatformInfo
	InstallDir        string
	ConfigPath        string
	Layers            []string
	ValidationTimeout time.Duration
}

type Result struct {
	Artifact   Artifact
	BinaryPath string
	ConfigPath string
}

type Downloader interface {
	Download(context.Context, string, io.Writer) error
}
type CommandRunner interface {
	Run(context.Context, string, ...string) CommandResult
}
type CommandResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Err      error
}

type HTTPDownloader struct{}
type OSCommandRunner struct{}
