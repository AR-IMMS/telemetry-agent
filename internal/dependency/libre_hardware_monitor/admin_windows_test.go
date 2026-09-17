//go:build windows

package librehardwaremonitor

import "testing"

func TestIsWindowsAdministratorResolvesCurrentProcessPrivilege(t *testing.T) {
	elevated, err := isWindowsAdministrator()
	if err != nil {
		t.Fatalf("isWindowsAdministrator() error = %v", err)
	}
	if !elevated {
		t.Skip("requires an Administrator terminal")
	}
}
