package identity

import (
	"runtime"
	"strings"
	"testing"
)

func TestCollectPlatformInfoIncludesHashedHostID(t *testing.T) {
	got, err := CollectPlatformInfo()
	if err != nil {
		t.Fatalf("CollectPlatformInfo() error = %v", err)
	}

	if strings.TrimSpace(got.HostID) == "" {
		t.Fatal("CollectPlatformInfo().HostID is empty")
	}

	if !strings.HasPrefix(got.HostID, runtime.GOOS+"-") {
		t.Fatalf(
			"CollectPlatformInfo().HostID = %q, want %q prefix",
			got.HostID,
			runtime.GOOS+"-",
		)
	}
}
