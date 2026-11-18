package cli

import (
	"context"
	"io"

	dependencycommands "github.com/ar-imms/telemetry-agent/internal/cli/commands/dependency"
	healthcommands "github.com/ar-imms/telemetry-agent/internal/cli/commands/health"
	inventorycommands "github.com/ar-imms/telemetry-agent/internal/cli/commands/inventory"
	monitoringcommands "github.com/ar-imms/telemetry-agent/internal/cli/commands/monitoring"
	"github.com/spf13/cobra"
)

// Dispatch forwards a parsed command path and its untouched arguments to the
// existing command handlers. Command-specific parsing remains in those handlers
// during the compatibility migration.
type Dispatch func(context.Context, []string, io.Writer, io.Writer) int

// Execute builds and runs the Cobra tree while preserving the compatibility
// root-help output and the established exit code from command handlers.
func Execute(ctx context.Context, args []string, stdout, stderr io.Writer, dispatch Dispatch, rootHelp func(io.Writer)) int {
	exitCode := 0
	if args == nil {
		args = []string{}
	}
	root := newRootCommand(dispatch, stdout, stderr, rootHelp, &exitCode)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		return dispatch(ctx, args, stdout, stderr)
	}
	return exitCode
}

func newRootCommand(dispatch Dispatch, stdout, stderr io.Writer, rootHelp func(io.Writer), exitCode *int) *cobra.Command {
	root := &cobra.Command{
		Use:           "agentctl",
		Short:         "AR-IMMS Telemetry Agent command line",
		SilenceErrors: true,
		SilenceUsage:  true,
		Run: func(command *cobra.Command, args []string) {
			*exitCode = dispatch(command.Context(), args, command.OutOrStdout(), command.ErrOrStderr())
		},
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetHelpCommand(&cobra.Command{
		Use:   "help",
		Short: "Show this help",
		Run: func(command *cobra.Command, _ []string) {
			rootHelp(command.OutOrStdout())
		},
	})
	defaultHelp := root.HelpFunc()
	root.SetHelpFunc(func(command *cobra.Command, args []string) {
		if command == root {
			rootHelp(command.OutOrStdout())
			return
		}
		defaultHelp(command, args)
	})

	for _, name := range []string{
		"bootstrap", "integration", "run",
		"discovery", "enroll", "install", "uninstall",
	} {
		root.AddCommand(newLegacyCommand(name, false, dispatch, exitCode))
	}
	root.AddCommand(healthcommands.NewCommands(dispatch, func(code int) {
		*exitCode = code
	})...)
	root.AddCommand(dependencycommands.NewCommand(dispatch, func(code int) {
		*exitCode = code
	}))
	root.AddCommand(inventorycommands.NewCommand(dispatch, func(code int) {
		*exitCode = code
	}))
	root.AddCommand(newLegacyCommand("service", true, dispatch, exitCode))
	root.AddCommand(monitoringcommands.NewCommand(dispatch, func(code int) {
		*exitCode = code
	}))
	return root
}

func newLegacyCommand(name string, hidden bool, dispatch Dispatch, exitCode *int) *cobra.Command {
	return &cobra.Command{
		Use:                name,
		DisableFlagParsing: true,
		Hidden:             hidden,
		Run: func(command *cobra.Command, args []string) {
			*exitCode = dispatch(command.Context(), append([]string{name}, args...), command.OutOrStdout(), command.ErrOrStderr())
		},
	}
}
