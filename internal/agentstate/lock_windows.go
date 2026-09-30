//go:build windows

package agentstate

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

type windowsStateLock struct {
	file       *os.File
	overlapped windows.Overlapped
}

func acquireStateLock(path string) (stateLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open Agent state lock: %w", err)
	}

	lock := windowsStateLock{
		file: file,
	}

	err = windows.LockFileEx(
		windows.Handle(file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK,
		0,
		1,
		0,
		&lock.overlapped,
	)
	if err != nil {
		file.Close()

		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, ErrStateLocked
		}

		return nil, fmt.Errorf("lock Agent state: %w", err)
	}

	return lock, nil
}

func (l windowsStateLock) Close() error {
	if err := windows.UnlockFileEx(
		windows.Handle(l.file.Fd()),
		0,
		1,
		0,
		&l.overlapped,
	); err != nil {
		l.file.Close()

		return fmt.Errorf("unlock Agent state: %w", err)
	}

	if err := l.file.Close(); err != nil {
		return fmt.Errorf("close Agent state lock: %w", err)
	}

	return nil
}
