//go:build windows

package dependency

import "testing"

func TestIsWindowsAdministratorResolvesCurrentProcessPrivilege(
	t *testing.T,
) {
	_, err := isWindowsAdministrator()

	if err != nil {
		t.Fatalf("isWindowsAdministrator() error = %v", err)
	}
}
