//go:build !linux && !windows

package agentstate

import "fmt"

func acquireStateLock(string) (stateLock, error) {
	return nil, fmt.Errorf(
		"Agent state locking is unsupported on this operating system",
	)
}
