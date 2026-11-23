package agentinstallation

import (
	"context"

	"github.com/ar-imms/telemetry-agent/internal/identity/host"
	servicedomain "github.com/ar-imms/telemetry-agent/internal/runtime/service/domain"
)

// Remover removes the persisted, Agent-owned installation resources.
type Remover interface {
	Remove(
		context.Context,
		servicedomain.AgentInstallation,
		string,
	) error
}

// NewRemover resolves the production Agent remover for a platform.
func NewRemover(
	platform identity.PlatformInfo,
) (Remover, error) {
	return newDefaultRemover(platform)
}
