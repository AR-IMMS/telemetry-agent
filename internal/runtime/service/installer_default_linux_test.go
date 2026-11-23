//go:build linux

package agentinstallation

import (
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/identity/host"
	"github.com/ar-imms/telemetry-agent/internal/runtime/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/testsupport/sqlitefixture"
)

func TestNewInstallerResolvesSystemdServiceOnLinux(t *testing.T) {
	installer := NewInstaller(
		bootstrap.HTTPDownloader{},
		bootstrap.OSCommandRunner{},
		sqlitefixture.NewStore,
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
