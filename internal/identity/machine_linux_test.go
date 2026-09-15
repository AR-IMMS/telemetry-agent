//go:build linux

package identity

import (
	"strings"
	"testing"
)

func TestCollectMachineIdentityReturnsCurrentLinuxMachine(t *testing.T) {
	got, err := CollectMachineIdentity()
	if err != nil {
		t.Fatalf("CollectMachineIdentity() error = %v", err)
	}

	if strings.TrimSpace(got.HostName) == "" {
		t.Fatal("CollectMachineIdentity().HostName is empty")
	}

	if !strings.HasPrefix(got.HostID, "linux-") {
		t.Fatalf(
			"CollectMachineIdentity().HostID = %q, want linux prefix",
			got.HostID,
		)
	}

	const hashLength = 24

	if len(got.HostID) != len("linux-")+hashLength {
		t.Fatalf(
			"CollectMachineIdentity().HostID length = %d, want %d",
			len(got.HostID),
			len("linux-")+hashLength,
		)
	}
}
