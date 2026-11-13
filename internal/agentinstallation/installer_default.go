package agentinstallation

import (
	"context"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/identity"
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
		installSupervisor: func(ctx context.Context, installDir string, platform identity.PlatformInfo) (string, error) {
			artifact, err := bootstrap.SelectSupervisorArtifact(platform)
			if err != nil {
				return "", err
			}
			result, err := bootstrap.InstallSupervisorBinary(ctx, installDir, artifact, downloader)
			if err != nil {
				return "", err
			}
			return result.BinaryPath, nil
		},
		newStateStore: func(path string) stateStore {
			return agentstate.NewStore(path)
		},
		serviceFor: newDefaultServiceInstaller,
	}
}
