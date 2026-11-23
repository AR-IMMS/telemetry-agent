//go:build !windows

package bootstrap

func isRetryablePublishRenameError(error) bool {
	return false
}
