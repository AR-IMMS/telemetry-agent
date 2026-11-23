//go:build windows

package main

import "golang.org/x/sys/windows"

func canPersistFleetCredentialForTest() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}
