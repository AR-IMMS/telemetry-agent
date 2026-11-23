//go:build !linux && !windows

package filelock

import "fmt"

func acquire(string) (Lock, error) {
	return nil, fmt.Errorf("file locking is unsupported on this operating system")
}
