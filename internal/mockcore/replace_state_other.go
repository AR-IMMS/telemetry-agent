//go:build !windows

package mockcore

import "os"

func replacePersistentState(source, destination string) error {
	return os.Rename(source, destination)
}
