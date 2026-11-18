//go:build windows

package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestActivateRenderedConfigPreservesExistingProtectedDACL(t *testing.T) {
	targetPath := filepath.Join(t.TempDir(), "collector.yaml")
	if err := os.WriteFile(targetPath, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	setTestProtectedDACL(t, targetPath)
	before := testDACLString(t, targetPath)

	if err := ActivateRenderedConfig(context.Background(), &activationRunner{result: CommandResult{}}, "/fake/collector", targetPath, []byte("new"), nil); err != nil {
		t.Fatal(err)
	}

	if after := testDACLString(t, targetPath); after != before {
		t.Fatalf("replacement DACL = %q, want preserved DACL %q", after, before)
	}
}

func TestActivateRenderedConfigPreservesExistingInheritedDACL(t *testing.T) {
	targetPath := filepath.Join(t.TempDir(), "collector.yaml")
	if err := os.WriteFile(targetPath, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := testDACLString(t, targetPath)

	if err := ActivateRenderedConfig(context.Background(), &activationRunner{result: CommandResult{}}, "/fake/collector", targetPath, []byte("new"), nil); err != nil {
		t.Fatal(err)
	}

	if after := testDACLString(t, targetPath); after != before {
		t.Fatalf("replacement DACL = %q, want preserved DACL %q", after, before)
	}
}

func setTestProtectedDACL(t *testing.T, path string) {
	t.Helper()
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		t.Fatal(err)
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}
}

func testDACLString(t *testing.T, path string) string {
	t.Helper()
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	return descriptor.String()
}
