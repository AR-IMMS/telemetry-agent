package bootstrap

import (
	"context"
	"io"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/config"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

const (
	collectorVersion = "0.160.0"

	maxArtifactBytes       int64 = 256 << 20
	maxExpandedArchiveSize int64 = 512 << 20
	maxCommandOutput             = 64 << 10
)

// Artifact describes a pinned Collector archive and the binary it must contain.
type Artifact struct {
	Version      string
	OS           string
	Architecture string

	URL              string
	ArchiveName      string
	ArchiveFormat    string
	ArchiveSHA256    string
	ArchiveSizeBytes int64

	// BinaryPath is the expected relative path inside the archive.
	BinaryPath string
	BinaryName string
}

// Options controls Collector installation and native configuration validation.
type Options struct {
	Platform identity.PlatformInfo

	// InstallDir is the parent directory that owns versions/.
	InstallDir string

	// ConfigPath must point to an already rendered Collector config.
	ConfigPath string

	ConfigInput config.RenderInput

	// ValidationEnvironment is supplied only to the validation subprocess.
	// Example: OTEL_GATEWAY_ENDPOINT=127.0.0.1:4317.
	ValidationEnvironment []string
	ValidationTimeout     time.Duration
}

// Result reports the verified Collector binary and configuration used by Run.
type Result struct {
	Artifact   Artifact
	BinaryPath string
	ConfigPath string
	Reused     bool
}

// Downloader writes a downloaded artifact to the supplied destination.
type Downloader interface {
	// Download retrieves url into dst while honoring ctx cancellation.
	Download(context.Context, string, io.Writer) error
}

// Command describes a Collector validation subprocess invocation.
type Command struct {
	Executable  string
	Args        []string
	Environment []string
}

// CommandRunner executes Collector subprocesses and returns bounded diagnostics.
type CommandRunner interface {
	// Run executes command and reports its exit status and captured output.
	Run(context.Context, Command) CommandResult
}

// CommandResult contains a subprocess exit status, diagnostics, and execution error.
type CommandResult struct {
	ExitCode        int
	Stdout          string
	Stderr          string
	OutputTruncated bool
	Err             error
}

// HTTPDownloader downloads artifacts over HTTP.
type HTTPDownloader struct{}

// OSCommandRunner runs Collector commands as operating-system processes.
type OSCommandRunner struct{}
