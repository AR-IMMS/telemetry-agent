//go:build windows

package application_test

import "golang.org/x/sys/windows"

func canPersistPlatformCredentialForTest() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}
