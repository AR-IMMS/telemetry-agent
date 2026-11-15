//go:build windows

package identity

import (
	"strings"
	"testing"
)

func TestCollectMachineIdentityReturnsCurrentWindowsMachine(t *testing.T) {
	got, err := CollectMachineIdentity()
	if err != nil {
		t.Fatalf("CollectMachineIdentity() error = %v", err)
	}

	if strings.TrimSpace(got.HostName) == "" {
		t.Fatal("CollectMachineIdentity().HostName is empty")
	}

	if !strings.HasPrefix(got.HostID, "windows-") {
		t.Fatalf(
			"CollectMachineIdentity().HostID = %q, want windows prefix",
			got.HostID,
		)
	}

	const hashLength = 24

	if len(got.HostID) != len("windows-")+hashLength {
		t.Fatalf(
			"CollectMachineIdentity().HostID length = %d, want %d",
			len(got.HostID),
			len("windows-")+hashLength,
		)
	}
}
