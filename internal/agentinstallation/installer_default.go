package agentinstallation

import (
	"context"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
)

// NewInstaller wires the production effects for a packaged Agent installation.
func NewInstaller(
	downloader bootstrap.Downloader,
	runner bootstrap.CommandRunner,
) Installer {
	return Installer{
		layoutFor:       DefaultLayout,
		copyAgentBinary: CopyAgentBinary,
		bootstrap: func(
			ctx context.Context,
			options bootstrap.Options,
		) (bootstrap.Result, error) {
			return bootstrap.Run(ctx, options, downloader, runner)
		},
		newStateStore: func(path string) stateStore {
			return agentstate.NewFileStore(path)
		},
		serviceFor: newDefaultServiceInstaller,
	}
}
