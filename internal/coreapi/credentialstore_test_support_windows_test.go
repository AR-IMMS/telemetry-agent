//go:build windows

package coreapi

import "golang.org/x/sys/windows"

func canPersistPlatformCredentialForTest() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}
