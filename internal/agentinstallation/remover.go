package agentinstallation

import (
	"context"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

// Remover removes the persisted, Agent-owned installation resources.
type Remover interface {
	Remove(
		context.Context,
		agentstate.AgentInstallation,
		string,
	) error
}

// NewRemover resolves the production Agent remover for a platform.
func NewRemover(
	platform identity.PlatformInfo,
) (Remover, error) {
	return newDefaultRemover(platform)
}
