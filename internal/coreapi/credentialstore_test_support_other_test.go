//go:build !windows

package coreapi

func canPersistPlatformCredentialForTest() bool { return true }
