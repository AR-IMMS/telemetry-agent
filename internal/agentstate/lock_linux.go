//go:build linux

package agentstate

import (
	"fmt"
	"os"
	"syscall"
)

type unixStateLock struct {
	file *os.File
}

func acquireStateLock(path string) (stateLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open Agent state lock: %w", err)
	}

	if err := syscall.Flock(
		int(file.Fd()),
		syscall.LOCK_EX,
	); err != nil {
		file.Close()

		if err != nil {
			file.Close()

			return nil, fmt.Errorf("lock Agent state: %w", err)
		}

		return nil, fmt.Errorf("lock Agent state: %w", err)
	}

	return unixStateLock{file: file}, nil
}

func (l unixStateLock) Close() error {
	if err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN); err != nil {
		l.file.Close()

		return fmt.Errorf("unlock Agent state: %w", err)
	}

	if err := l.file.Close(); err != nil {
		return fmt.Errorf("close Agent state lock: %w", err)
	}

	return nil
}
