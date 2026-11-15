package identity

import "testing"

func TestCollectPlatformInfoProvidesNormalizedHostFacts(t *testing.T) {
	info, err := CollectPlatformInfo()
	if err != nil {
		t.Fatalf("CollectPlatformInfo() error = %v", err)
	}
	if info.OS == "" || info.Architecture == "" {
		t.Fatalf("platform facts = %#v, want non-empty OS and architecture", info)
	}
}
