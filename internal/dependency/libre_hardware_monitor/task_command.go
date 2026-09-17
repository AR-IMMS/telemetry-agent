package librehardwaremonitor

import (
	"fmt"
	"strings"
)

// processCommand describes one operating-system process without running it.
type processCommand struct {
	Executable string
	Args       []string
}

// buildTaskCreateCommand creates the overwrite-safe scheduled-task command.
func buildTaskCreateCommand(
	taskName string,
	taskXMLPath string,
) (processCommand, error) {
	taskName = strings.TrimSpace(taskName)
	taskXMLPath = strings.TrimSpace(taskXMLPath)

	if taskName == "" {
		return processCommand{}, fmt.Errorf(
			"Libre Hardware Monitor task name is required",
		)
	}
	if taskXMLPath == "" {
		return processCommand{}, fmt.Errorf(
			"Libre Hardware Monitor task XML path is required",
		)
	}

	return processCommand{
		Executable: "schtasks.exe",
		Args: []string{
			"/Create",
			"/TN",
			taskName,
			"/XML",
			taskXMLPath,
			"/F",
		},
	}, nil
}

// buildTaskRunCommand starts the managed task without waiting for next boot.
func buildTaskRunCommand(
	taskName string,
) (processCommand, error) {
	taskName = strings.TrimSpace(taskName)

	if taskName == "" {
		return processCommand{}, fmt.Errorf(
			"Libre Hardware Monitor task name is required",
		)
	}

	return processCommand{
		Executable: "schtasks.exe",
		Args: []string{
			"/Run",
			"/TN",
			taskName,
		},
	}, nil
}
