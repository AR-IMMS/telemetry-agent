package main

import (
	"context"
	"io"

	"github.com/spf13/cobra"
)

// executeAgentctl builds the command tree while forwarding every existing command
// to its compatibility handler. The new Monitoring paths are aliases only.
func executeAgentctl(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	deps dependencies,
) int {
	exitCode := 0
	root := &cobra.Command{
		Use:           "agentctl",
		Short:         "AR-IMMS Telemetry Agent command line",
		SilenceErrors: true,
		SilenceUsage:  true,
		Run: func(_ *cobra.Command, commandArgs []string) {
			exitCode = runLegacy(ctx, commandArgs, stdout, stderr, deps)
		},
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetHelpCommand(&cobra.Command{
		Use:   "help",
		Short: "Show this help",
		Run: func(_ *cobra.Command, _ []string) {
			writeRootHelp(stdout)
		},
	})
	defaultHelp := root.HelpFunc()
	root.SetHelpFunc(func(command *cobra.Command, args []string) {
		if command == root {
			writeRootHelp(stdout)
			return
		}
		defaultHelp(command, args)
	})

	for _, name := range []string{
		"bootstrap", "dependency", "integration", "run", "inventory",
		"discovery", "status", "doctor", "enroll", "install", "uninstall",
	} {
		root.AddCommand(newLegacyCommand(ctx, name, stdout, stderr, deps, false, &exitCode))
	}
	root.AddCommand(newLegacyCommand(ctx, "service", stdout, stderr, deps, true, &exitCode))
	root.AddCommand(newMonitoringCommand(ctx, stdout, stderr, deps, &exitCode))

	if args == nil {
		args = []string{}
	}
	root.SetArgs(args)
	if _, err := root.ExecuteC(); err != nil {
		// Keep the established diagnostic and root-help shape for unknown commands.
		return runLegacy(ctx, args, stdout, stderr, deps)
	}
	return exitCode
}

func newLegacyCommand(
	ctx context.Context,
	name string,
	stdout io.Writer,
	stderr io.Writer,
	deps dependencies,
	hidden bool,
	exitCode *int,
) *cobra.Command {
	return &cobra.Command{
		Use:                name,
		DisableFlagParsing: true,
		Hidden:             hidden,
		Run: func(_ *cobra.Command, args []string) {
			*exitCode = runLegacy(ctx, append([]string{name}, args...), stdout, stderr, deps)
		},
	}
}

func newMonitoringCommand(
	ctx context.Context,
	stdout io.Writer,
	stderr io.Writer,
	deps dependencies,
	exitCode *int,
) *cobra.Command {
	monitoring := &cobra.Command{
		Use:   "monitoring",
		Short: "Manage monitoring instances, templates, and proposals",
		Run: func(command *cobra.Command, _ []string) {
			_ = command.Help()
		},
	}
	instances := &cobra.Command{
		Use:   "instance",
		Short: "Manage desired monitoring instances",
		Run: func(command *cobra.Command, _ []string) {
			_ = command.Help()
		},
	}
	templates := &cobra.Command{
		Use:   "template",
		Short: "Manage local monitoring templates",
		Run: func(command *cobra.Command, _ []string) {
			_ = command.Help()
		},
	}
	proposals := &cobra.Command{
		Use:   "proposal",
		Short: "Review monitoring proposals",
		Run: func(command *cobra.Command, _ []string) {
			_ = command.Help()
		},
	}
	profiles := &cobra.Command{
		Use:   "profile",
		Short: "Inspect built-in monitoring profiles",
		Run: func(command *cobra.Command, _ []string) {
			_ = command.Help()
		},
	}

	for _, operation := range []string{
		"configure", "update", "enable", "disable", "delete", "list", "inspect",
		"status", "history", "retry",
	} {
		instances.AddCommand(newAliasCommand(operation, exitCode, func(args []string) int {
			return runLegacy(ctx, append([]string{"integration", operation}, args...), stdout, stderr, deps)
		}))
	}
	for _, operation := range []string{"create", "update", "list", "inspect", "enable", "disable", "delete"} {
		templates.AddCommand(newAliasCommand(operation, exitCode, func(args []string) int {
			return runLegacy(ctx, append([]string{"integration", "template", operation}, args...), stdout, stderr, deps)
		}))
	}
	for _, operation := range []string{"list", "inspect", "dismiss", "approve"} {
		proposals.AddCommand(newAliasCommand(operation, exitCode, func(args []string) int {
			return runLegacy(ctx, append([]string{"discovery", "proposals", operation}, args...), stdout, stderr, deps)
		}))
	}
	for _, operation := range []string{"list", "inspect"} {
		profiles.AddCommand(newAliasCommand(operation, exitCode, func(args []string) int {
			return runLegacy(ctx, append([]string{"integration", "catalog", operation}, args...), stdout, stderr, deps)
		}))
	}

	monitoring.AddCommand(instances, profiles, templates, proposals)
	return monitoring
}

func newAliasCommand(operation string, exitCode *int, execute func([]string) int) *cobra.Command {
	return &cobra.Command{
		Use:                operation,
		DisableFlagParsing: true,
		Run: func(_ *cobra.Command, args []string) {
			*exitCode = execute(args)
		},
	}
}
