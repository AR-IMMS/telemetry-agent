package librehardwaremonitor

import (
	"context"
	"fmt"
	"os"
)

// processRunner executes a prepared operating-system process.
type processRunner func(context.Context, processCommand) error

// RegisterTask renders a temporary task definition, replaces the managed task,
// starts it immediately, and removes the temporary XML file.
func RegisterTask(
	ctx context.Context,
	options Options,
	interactiveUserSID string,
	run processRunner,
) error {
	if run == nil {
		return fmt.Errorf("Libre Hardware Monitor process runner is required")
	}

	rendered, err := RenderTaskXML(options, interactiveUserSID)
	if err != nil {
		return err
	}

	taskFile, err := os.CreateTemp("", ".ar-imms-lhm-task-*.xml")
	if err != nil {
		return fmt.Errorf(
			"create Libre Hardware Monitor task XML file: %w",
			err,
		)
	}

	taskXMLPath := taskFile.Name()
	defer os.Remove(taskXMLPath)

	if _, err := taskFile.Write(rendered); err != nil {
		taskFile.Close()

		return fmt.Errorf(
			"write Libre Hardware Monitor task XML file: %w",
			err,
		)
	}

	if err := taskFile.Close(); err != nil {
		return fmt.Errorf(
			"close Libre Hardware Monitor task XML file: %w",
			err,
		)
	}

	createCommand, err := buildTaskCreateCommand(
		options.TaskName,
		taskXMLPath,
	)
	if err != nil {
		return err
	}

	if err := run(ctx, createCommand); err != nil {
		return fmt.Errorf(
			"create Libre Hardware Monitor scheduled task: %w",
			err,
		)
	}

	runCommand, err := buildTaskRunCommand(options.TaskName)
	if err != nil {
		return err
	}

	if err := run(ctx, runCommand); err != nil {
		return fmt.Errorf(
			"start Libre Hardware Monitor scheduled task: %w",
			err,
		)
	}

	return nil
}
