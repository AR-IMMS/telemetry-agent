//go:build !windows

package credentialstore

func canPersistPlatformCredentialForTest() bool { return true }
