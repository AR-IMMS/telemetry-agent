package dependency

import (
	"context"
	"io"

	"github.com/spf13/cobra"
)

// NewCommand builds the existing dependency command group. It forwards the
// untouched legacy arguments, context, streams, and exit code to the caller.
func NewCommand(
	dispatch func(context.Context, []string, io.Writer, io.Writer) int,
	setExitCode func(int),
) *cobra.Command {
	group := &cobra.Command{
		Use:                "dependency",
		Short:              "Existing dependency commands",
		DisableFlagParsing: true,
		Run: func(command *cobra.Command, args []string) {
			if hasHelpArgument(args) {
				_ = command.Help()
				return
			}
			setExitCode(dispatch(command.Context(), append([]string{"dependency"}, args...), command.OutOrStdout(), command.ErrOrStderr()))
		},
	}
	for _, operation := range []string{"list", "install", "status", "pending", "configure", "disable", "uninstall"} {
		legacyOperation := operation
		group.AddCommand(&cobra.Command{
			Use:                legacyOperation,
			DisableFlagParsing: true,
			Run: func(command *cobra.Command, args []string) {
				if hasHelpArgument(args) {
					_ = command.Help()
					return
				}
				setExitCode(dispatch(command.Context(), append([]string{"dependency", legacyOperation}, args...), command.OutOrStdout(), command.ErrOrStderr()))
			},
		})
	}
	return group
}

func hasHelpArgument(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}
