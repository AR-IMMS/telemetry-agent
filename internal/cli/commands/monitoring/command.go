package monitoring

import (
	"context"
	"io"

	"github.com/spf13/cobra"
)

// NewCommand creates the Monitoring alias tree. Each alias forwards the same
// arguments to the existing handler and reports its exit code to the caller.
func NewCommand(
	dispatch func(context.Context, []string, io.Writer, io.Writer) int,
	setExitCode func(int),
) *cobra.Command {
	monitoring := &cobra.Command{
		Use:   "monitoring",
		Short: "Manage monitoring instances, templates, and proposals",
		Run: func(command *cobra.Command, _ []string) {
			_ = command.Help()
		},
	}
	instances := &cobra.Command{Use: "instance", Short: "Manage desired monitoring instances"}
	templates := &cobra.Command{Use: "template", Short: "Manage local monitoring templates"}
	proposals := &cobra.Command{Use: "proposal", Short: "Review monitoring proposals"}
	profiles := &cobra.Command{Use: "profile", Short: "Inspect built-in monitoring profiles"}
	for _, operation := range []string{
		"create", "configure", "update", "enable", "disable", "delete", "list", "inspect", "match",
		"status", "history", "retry",
	} {
		legacyOperation := operation
		if operation == "create" {
			legacyOperation = "configure"
		}
		instances.AddCommand(newAliasCommand(operation, setExitCode, func(ctx context.Context, args []string, stdout, stderr io.Writer) int {
			return dispatch(ctx, append([]string{"integration", legacyOperation}, args...), stdout, stderr)
		}))
	}
	for _, operation := range []string{"create", "update", "list", "inspect", "enable", "disable", "delete"} {
		legacyOperation := operation
		templates.AddCommand(newAliasCommand(operation, setExitCode, func(ctx context.Context, args []string, stdout, stderr io.Writer) int {
			return dispatch(ctx, append([]string{"integration", "template", legacyOperation}, args...), stdout, stderr)
		}))
	}
	for _, operation := range []string{"list", "inspect", "dismiss", "approve"} {
		legacyOperation := operation
		proposals.AddCommand(newAliasCommand(operation, setExitCode, func(ctx context.Context, args []string, stdout, stderr io.Writer) int {
			return dispatch(ctx, append([]string{"discovery", "proposals", legacyOperation}, args...), stdout, stderr)
		}))
	}
	for _, operation := range []string{"list", "inspect"} {
		legacyOperation := operation
		profiles.AddCommand(newAliasCommand(operation, setExitCode, func(ctx context.Context, args []string, stdout, stderr io.Writer) int {
			return dispatch(ctx, append([]string{"integration", "catalog", legacyOperation}, args...), stdout, stderr)
		}))
	}
	monitoring.AddCommand(instances, profiles, templates, proposals)
	return monitoring
}

func newAliasCommand(
	operation string,
	setExitCode func(int),
	execute func(context.Context, []string, io.Writer, io.Writer) int,
) *cobra.Command {
	return &cobra.Command{
		Use:                operation,
		DisableFlagParsing: true,
		Run: func(command *cobra.Command, args []string) {
			setExitCode(execute(command.Context(), args, command.OutOrStdout(), command.ErrOrStderr()))
		},
	}
}
