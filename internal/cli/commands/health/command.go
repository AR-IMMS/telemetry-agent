package health

import (
	"context"
	"io"

	"github.com/spf13/cobra"
)

// NewCommands builds the existing top-level Health commands. Each command
// forwards its unchanged arguments, context, streams, and exit code.
func NewCommands(
	dispatch func(context.Context, []string, io.Writer, io.Writer) int,
	setExitCode func(int),
) []*cobra.Command {
	commands := make([]*cobra.Command, 0, 2)
	for _, name := range []string{"status", "doctor"} {
		commandName := name
		commands = append(commands, &cobra.Command{
			Use:                commandName,
			DisableFlagParsing: true,
			Run: func(command *cobra.Command, args []string) {
				setExitCode(dispatch(command.Context(), append([]string{commandName}, args...), command.OutOrStdout(), command.ErrOrStderr()))
			},
		})
	}
	return commands
}
