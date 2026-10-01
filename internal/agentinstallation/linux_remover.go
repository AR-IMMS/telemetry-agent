package agentinstallation

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

// linuxAgentRemover removes only Agent-owned Linux service and file resources.
type linuxAgentRemover struct {
	removeService   func(context.Context, string, string) error
	removeFile      func(string) error
	removeDirectory func(string) error
}

func (r linuxAgentRemover) Remove(
	ctx context.Context,
	installation agentstate.AgentInstallation,
	statePath string,
) error {
	if ctx == nil {
		return fmt.Errorf("Linux Agent removal context is required")
	}
	if !strings.EqualFold(
		strings.TrimSpace(installation.Platform),
		"linux",
	) {
		return fmt.Errorf(
			"Linux Agent remover does not support platform %q",
			installation.Platform,
		)
	}
	if strings.TrimSpace(installation.ServiceName) == "" {
		return fmt.Errorf("Agent systemd service name is required")
	}
	if strings.TrimSpace(statePath) == "" {
		return fmt.Errorf("Agent state path is required")
	}
	if r.removeService == nil {
		return fmt.Errorf("Agent systemd service remover is required")
	}
	if r.removeFile == nil {
		return fmt.Errorf("Agent file remover is required")
	}
	if r.removeDirectory == nil {
		return fmt.Errorf("Agent directory remover is required")
	}

	unitPath := path.Join(
		"/etc/systemd/system",
		installation.ServiceName+".service",
	)

	ownsService := false
	ownsUnit := false
	var files []string
	var directories []string
	var dataDirectories []string

	for _, resource := range installation.Ownership.Resources {
		switch resource.Kind {
		case "systemd-service":
			if resource.Identifier == installation.ServiceName {
				ownsService = true
			}

		case "file":
			if resource.Identifier == unitPath {
				ownsUnit = true
				continue
			}

			files = append(files, resource.Identifier)

		case "directory":
			if containsPath(resource.Identifier, statePath) {
				dataDirectories = append(dataDirectories, resource.Identifier)
				continue
			}

			directories = append(directories, resource.Identifier)
		}
	}

	if !ownsService {
		return fmt.Errorf(
			"Agent ownership does not include systemd service %q",
			installation.ServiceName,
		)
	}
	if !ownsUnit {
		return fmt.Errorf(
			"Agent ownership does not include systemd unit %q",
			unitPath,
		)
	}
	if len(dataDirectories) != 1 {
		return fmt.Errorf(
			"Agent ownership must include exactly one data directory containing %q",
			statePath,
		)
	}

	if err := r.removeService(
		ctx,
		installation.ServiceName,
		unitPath,
	); err != nil {
		return fmt.Errorf("remove Agent systemd service: %w", err)
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

func containsPath(directory string, path string) bool {
	relativePath, err := filepath.Rel(directory, path)
	if err != nil {
		return false
	}

	return relativePath != ".." &&
		!strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) &&
		!filepath.IsAbs(relativePath)
}
