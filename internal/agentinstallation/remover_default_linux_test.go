//go:build linux

package agentinstallation

import (
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/identity"
)

func TestNewRemoverResolvesLinuxFinalizer(t *testing.T) {
	remover, err := NewRemover(identity.PlatformInfo{
		OS: "linux",
	})
	if err != nil {
		t.Fatalf("NewRemover() error = %v", err)
	}

	if _, ok := remover.(linuxAgentRemover); !ok {
		t.Fatalf("remover = %T, want linuxAgentRemover", remover)
	}
}
