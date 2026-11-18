//go:build windows

package identity

import "testing"

func TestEnvironmentVersionEnrichmentDoesNotChangeLegacyPlatformDetails(t *testing.T) {
	legacy, err := collectPlatformDetails()
	if err != nil {
		t.Fatalf("collectPlatformDetails() error = %v", err)
	}
	if legacy.Version != "" {
		t.Fatalf("legacy platform version = %q, want empty for compatibility", legacy.Version)
	}
}
