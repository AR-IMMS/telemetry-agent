//go:build !windows

package credentials

func canPersistPlatformCredentialForTest() bool { return true }
