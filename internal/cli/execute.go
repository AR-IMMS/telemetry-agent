package cli

import (
	"context"
	"io"

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
		"discovery", "status", "doctor", "enroll", "install", "uninstall",
	} {
		root.AddCommand(newLegacyCommand(name, false, dispatch, exitCode))
	}
	root.AddCommand(newLegacyOperationsCommand("dependency", []string{
		"list", "install", "status", "pending", "configure", "disable", "uninstall",
	}, dispatch, exitCode))
	root.AddCommand(newLegacyOperationsCommand("inventory", []string{
		"list", "inspect", "status",
	}, dispatch, exitCode))
	root.AddCommand(newLegacyCommand("service", true, dispatch, exitCode))
	root.AddCommand(newMonitoringCommand(dispatch, exitCode))
	return root
}

func newLegacyOperationsCommand(name string, operations []string, dispatch Dispatch, exitCode *int) *cobra.Command {
	group := &cobra.Command{
		Use:                name,
		Short:              "Existing " + name + " commands",
		DisableFlagParsing: true,
		Run: func(command *cobra.Command, args []string) {
			if hasHelpArgument(args) {
				_ = command.Help()
				return
			}
			*exitCode = dispatch(command.Context(), append([]string{name}, args...), command.OutOrStdout(), command.ErrOrStderr())
		},
	}
	for _, operation := range operations {
		group.AddCommand(&cobra.Command{
			Use:                operation,
			DisableFlagParsing: true,
			Run: func(command *cobra.Command, args []string) {
				if hasHelpArgument(args) {
					_ = command.Help()
					return
				}
				*exitCode = dispatch(command.Context(), append([]string{name, operation}, args...), command.OutOrStdout(), command.ErrOrStderr())
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

func newMonitoringCommand(dispatch Dispatch, exitCode *int) *cobra.Command {
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
		"configure", "update", "enable", "disable", "delete", "list", "inspect",
		"status", "history", "retry",
	} {
		instances.AddCommand(newAliasCommand(operation, exitCode, func(ctx context.Context, args []string, stdout, stderr io.Writer) int {
			return dispatch(ctx, append([]string{"integration", operation}, args...), stdout, stderr)
		}))
	}
	for _, operation := range []string{"create", "update", "list", "inspect", "enable", "disable", "delete"} {
		templates.AddCommand(newAliasCommand(operation, exitCode, func(ctx context.Context, args []string, stdout, stderr io.Writer) int {
			return dispatch(ctx, append([]string{"integration", "template", operation}, args...), stdout, stderr)
		}))
	}
	for _, operation := range []string{"list", "inspect", "dismiss", "approve"} {
		proposals.AddCommand(newAliasCommand(operation, exitCode, func(ctx context.Context, args []string, stdout, stderr io.Writer) int {
			return dispatch(ctx, append([]string{"discovery", "proposals", operation}, args...), stdout, stderr)
		}))
	}
	for _, operation := range []string{"list", "inspect"} {
		profiles.AddCommand(newAliasCommand(operation, exitCode, func(ctx context.Context, args []string, stdout, stderr io.Writer) int {
			return dispatch(ctx, append([]string{"integration", "catalog", operation}, args...), stdout, stderr)
		}))
	}
	monitoring.AddCommand(instances, profiles, templates, proposals)
	return monitoring
}

type aliasHandler func(context.Context, []string, io.Writer, io.Writer) int

func newAliasCommand(operation string, exitCode *int, execute aliasHandler) *cobra.Command {
	return &cobra.Command{
		Use:                operation,
		DisableFlagParsing: true,
		Run: func(command *cobra.Command, args []string) {
			*exitCode = execute(command.Context(), args, command.OutOrStdout(), command.ErrOrStderr())
		},
	}
}
