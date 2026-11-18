//go:build windows

package application_test

import "golang.org/x/sys/windows"

func canUseProtectedCredentialStoreForTest() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}
