package librehardwaremonitor

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// scheduledTaskInspector reports whether the named Task Scheduler task exists.
type scheduledTaskInspector func(
	context.Context,
	string,
) (bool, error)

// scheduledTaskMatcher reports whether the exported task definition matches
// the Agent-owned startup and LocalSystem contract.
type scheduledTaskMatcher func(
	context.Context,
	Options,
) (bool, error)

// configurationInspector reports whether required Agent-owned settings match.
type configurationInspector func(Options) (bool, error)

// firewallInspector reports whether the managed firewall rule matches.
type firewallInspector func(context.Context, Options) (bool, error)

// healthInspector checks the local LHM metrics endpoint.
type healthInspector func(context.Context) error

// inspectInstallation reads the Agent-owned LHM state without changing Windows.
func inspectInstallation(
	ctx context.Context,
	options Options,
	inspectTask scheduledTaskInspector,
	matchTask scheduledTaskMatcher,
	inspectConfig configurationInspector,
	inspectFirewall firewallInspector,
	checkHealth healthInspector,
) (installationState, error) {
	if err := options.Validate(); err != nil {
		return installationState{}, fmt.Errorf(
			"validate Libre Hardware Monitor inspection options: %w",
			err,
		)
	}
	if inspectTask == nil {
		return installationState{}, fmt.Errorf(
			"Libre Hardware Monitor task inspector is required",
		)
	}
	if matchTask == nil {
		return installationState{}, fmt.Errorf(
			"Libre Hardware Monitor task matcher is required",
		)
	}
	if inspectConfig == nil {
		return installationState{}, fmt.Errorf(
			"Libre Hardware Monitor configuration inspector is required",
		)
	}
	if inspectFirewall == nil {
		return installationState{}, fmt.Errorf(
			"Libre Hardware Monitor firewall inspector is required",
		)
	}
	if checkHealth == nil {
		return installationState{}, fmt.Errorf(
			"Libre Hardware Monitor health inspector is required",
		)
	}

	taskExists, err := inspectTask(ctx, options.TaskName)
	if err != nil {
		return installationState{}, fmt.Errorf(
			"inspect Libre Hardware Monitor task: %w",
			err,
		)
	}

	if !taskExists {
		return installationState{}, nil
	}

	taskMatches, err := matchTask(ctx, options)
	if err != nil {
		return installationState{}, fmt.Errorf(
			"inspect Libre Hardware Monitor task definition: %w",
			err,
		)
	}

	configMatches, err := inspectConfig(options)
	if err != nil {
		return installationState{}, fmt.Errorf(
			"inspect Libre Hardware Monitor configuration: %w",
			err,
		)
	}

	firewallMatches, err := inspectFirewall(ctx, options)
	if err != nil {
		return installationState{}, fmt.Errorf(
			"inspect Libre Hardware Monitor firewall: %w",
			err,
		)
	}

	state := installationState{
		TaskExists:      true,
		ConfigMatches:   configMatches,
		FirewallMatches: firewallMatches,

		// Existence is the only task state inspected in this first adapter.
		// XML-definition drift is added with the Task Scheduler adapter.
		TaskMatches: taskMatches,
	}

	if err := checkHealth(ctx); err == nil {
		state.Healthy = true
	}

	return state, nil
}

// newInstallationInspector composes the live Task Scheduler, configuration,
// firewall, and local-metrics checks used by the production installer.
func newInstallationInspector(
	options Options,
	client *http.Client,
	healthPollInterval time.Duration,
	run processOutputRunner,
) installationInspector {
	return func(ctx context.Context) (installationState, error) {
		healthEndpoint := "http://127.0.0.1:" +
			strconv.Itoa(options.ListenPort) +
			"/metrics"

		return inspectInstallation(
			ctx,
			options,
			func(
				ctx context.Context,
				taskName string,
			) (bool, error) {
				return scheduledTaskExists(ctx, taskName, run)
			},
			func(
				ctx context.Context,
				options Options,
			) (bool, error) {
				return scheduledTaskMatches(ctx, options, run)
			},
			ConfigMatches,
			func(
				ctx context.Context,
				options Options,
			) (bool, error) {
				return firewallMatches(ctx, options, run)
			},
			func(ctx context.Context) error {
				return WaitForHealth(
					ctx,
					client,
					healthEndpoint,
					healthPollInterval,
				)
			},
		)
	}
}
