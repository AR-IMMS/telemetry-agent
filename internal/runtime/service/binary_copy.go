package agentinstallation

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// CopyAgentBinary atomically copies one regular executable file into the
// Agent-owned installation directory.
func CopyAgentBinary(
	ctx context.Context,
	sourcePath string,
	destinationPath string,
) error {
	if ctx == nil {
		return fmt.Errorf("Agent binary copy context is required")
	}

	sourcePath = strings.TrimSpace(sourcePath)
	if sourcePath == "" {
		return fmt.Errorf("Agent source binary path is required")
	}

	destinationPath = strings.TrimSpace(destinationPath)
	if destinationPath == "" {
		return fmt.Errorf("Agent destination binary path is required")
	}

	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open Agent source binary: %w", err)
	}
	defer source.Close()

	sourceInfo, err := source.Stat()
	if err != nil {
		return fmt.Errorf("stat Agent source binary: %w", err)
	}
	if !sourceInfo.Mode().IsRegular() {
		return fmt.Errorf(
			"Agent source binary %q is not a regular file",
			sourcePath,
		)
	}

	destinationDirectory := filepath.Dir(destinationPath)
	if err := os.MkdirAll(destinationDirectory, 0o755); err != nil {
		return fmt.Errorf(
			"create Agent installation directory: %w",
			err,
		)
	}

	temporary, err := os.CreateTemp(
		destinationDirectory,
		".agentctl-*",
	)
	if err != nil {
		return fmt.Errorf("create temporary Agent binary: %w", err)
	}

	temporaryPath := temporary.Name()
	cleanup := true

	defer func() {
		if cleanup {
			_ = temporary.Close()
			_ = os.Remove(temporaryPath)
		}
	}()

	if err := temporary.Chmod(sourceInfo.Mode().Perm()); err != nil {
		return fmt.Errorf("set temporary Agent binary mode: %w", err)
	}

	if err := copyWithContext(ctx, temporary, source); err != nil {
		return fmt.Errorf("copy Agent binary: %w", err)
	}

	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary Agent binary: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary Agent binary: %w", err)
	}

	if err := os.Rename(temporaryPath, destinationPath); err != nil {
		return fmt.Errorf("activate Agent binary: %w", err)
	}

	cleanup = false

	return nil
}

func copyWithContext(
	ctx context.Context,
	destination io.Writer,
	source io.Reader,
) error {
	buffer := make([]byte, 32*1024)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		readCount, readErr := source.Read(buffer)

		if readCount > 0 {
			if _, err := destination.Write(buffer[:readCount]); err != nil {
				return err
			}
		}

		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}
