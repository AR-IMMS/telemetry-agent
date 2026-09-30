//go:build linux

package agentinstallation

import (
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

func TestNewInstallerResolvesSystemdServiceOnLinux(t *testing.T) {
	installer := NewInstaller(
		bootstrap.HTTPDownloader{},
		bootstrap.OSCommandRunner{},
	)

	if installer.serviceFor == nil {
		t.Fatal("service resolver = nil")
	}

	layout, err := DefaultLayout(identity.PlatformInfo{
		OS: "linux",
	})
	if err != nil {
		t.Fatalf("DefaultLayout() error = %v", err)
	}

	service, err := installer.serviceFor(layout)
	if err != nil {
		t.Fatalf("serviceFor() error = %v", err)
	}

	if _, ok := service.(systemdServiceInstaller); !ok {
		t.Fatalf("service = %T, want systemdServiceInstaller", service)
	}
}
