//go:build windows

package credentialstore

import "golang.org/x/sys/windows"

func canPersistPlatformCredentialForTest() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}
