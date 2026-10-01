package agentinstallation

import (
	"context"
	"fmt"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

// windowsAgentRemover removes only Agent-owned Windows service and paths.
type windowsAgentRemover struct {
	removeService   func(context.Context, string) error
	removeFile      func(string) error
	removeDirectory func(string) error
}

func (r windowsAgentRemover) Remove(
	ctx context.Context,
	installation agentstate.AgentInstallation,
	statePath string,
) error {
	if ctx == nil {
		return fmt.Errorf("Windows Agent removal context is required")
	}
	if !strings.EqualFold(
		strings.TrimSpace(installation.Platform),
		"windows",
	) {
		return fmt.Errorf(
			"Windows Agent remover does not support platform %q",
			installation.Platform,
		)
	}
	if strings.TrimSpace(installation.ServiceName) == "" {
		return fmt.Errorf("Agent Windows service name is required")
	}
	if strings.TrimSpace(statePath) == "" {
		return fmt.Errorf("Agent state path is required")
	}
	if r.removeService == nil {
		return fmt.Errorf("Agent Windows service remover is required")
	}
	if r.removeFile == nil {
		return fmt.Errorf("Agent file remover is required")
	}
	if r.removeDirectory == nil {
		return fmt.Errorf("Agent directory remover is required")
	}

	ownsService := false
	var files []string
	var directories []string
	var dataDirectories []string

	for _, resource := range installation.Ownership.Resources {
		switch resource.Kind {
		case "windows-service":
			if resource.Identifier == installation.ServiceName {
				ownsService = true
			}

		case "file":
			files = append(files, resource.Identifier)

		case "directory":
			if containsWindowsPath(resource.Identifier, statePath) {
				dataDirectories = append(dataDirectories, resource.Identifier)
				continue
			}

			directories = append(directories, resource.Identifier)
		}
	}

	if !ownsService {
		return fmt.Errorf(
			"Agent ownership does not include Windows service %q",
			installation.ServiceName,
		)
	}
	if len(dataDirectories) != 1 {
		return fmt.Errorf(
			"Agent ownership must include exactly one data directory containing %q",
			statePath,
		)
	}

	if err := r.removeService(ctx, installation.ServiceName); err != nil {
		return fmt.Errorf("remove Agent Windows service: %w", err)
	}

	for _, path := range files {
		if err := r.removeFile(path); err != nil {
			return fmt.Errorf("remove Agent-owned file %q: %w", path, err)
		}
	}

	for _, path := range directories {
		if err := r.removeDirectory(path); err != nil {
			return fmt.Errorf(
				"remove Agent-owned directory %q: %w",
				path,
				err,
			)
		}
	}

	// The data directory contains state.json, so it is removed last.
	if err := r.removeDirectory(dataDirectories[0]); err != nil {
		return fmt.Errorf(
			"remove Agent-owned data directory %q: %w",
			dataDirectories[0],
			err,
		)
	}

	return nil
}

func containsWindowsPath(directory string, path string) bool {
	directory = strings.TrimRight(
		strings.ReplaceAll(strings.TrimSpace(directory), `\`, "/"),
		"/",
	)
	path = strings.ReplaceAll(strings.TrimSpace(path), `\`, "/")

	return strings.EqualFold(path, directory) ||
		strings.HasPrefix(
			strings.ToLower(path),
			strings.ToLower(directory)+"/",
		)
}
