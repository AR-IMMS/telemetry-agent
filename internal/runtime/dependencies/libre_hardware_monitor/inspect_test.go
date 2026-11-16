package librehardwaremonitor

import (
	"context"
	"testing"
)

func TestInspectInstallationReportsHealthyMatchingTask(t *testing.T) {
	state, err := inspectInstallation(
		context.Background(),
		DefaultOptions(),

		// inspectTask
		func(
			ctx context.Context,
			taskName string,
		) (bool, error) {
			if taskName != "AR-IMMS-LibreHardwareMonitor" {
				t.Fatalf("task name = %q", taskName)
			}

			return true, nil
		},

		// inspectTaskEnabled
		func(
			ctx context.Context,
			taskName string,
		) (bool, error) {
			return true, nil
		},

		// matchTask
		func(
			ctx context.Context,
			options Options,
		) (bool, error) {
			return true, nil
		},

		// inspectConfig
		func(options Options) (bool, error) {
			return true, nil
		},

		// inspectFirewall
		func(
			ctx context.Context,
			options Options,
		) (bool, error) {
			return true, nil
		},

		// checkHealth
		func(ctx context.Context) error {
			return nil
		},
	)
	if err != nil {
		t.Fatalf("inspectInstallation() error = %v", err)
	}

	want := installationState{
		TaskExists:      true,
		TaskEnabled:     true,
		Healthy:         true,
		ConfigMatches:   true,
		FirewallMatches: true,
		TaskMatches:     true,
	}

	if state != want {
		t.Fatalf("installation state = %+v, want %+v", state, want)
	}
}

func TestInspectInstallationSkipsOtherChecksWhenTaskIsAbsent(t *testing.T) {
	state, err := inspectInstallation(
		context.Background(),
		DefaultOptions(),

		// inspectTask
		func(
			ctx context.Context,
			taskName string,
		) (bool, error) {
			return false, nil
		},

		// inspectTaskEnabled
		func(
			ctx context.Context,
			taskName string,
		) (bool, error) {
			t.Fatal("task enabled inspection must not run when task is absent")

			return false, nil
		},

		// matchTask
		func(
			ctx context.Context,
			options Options,
		) (bool, error) {
			t.Fatal("task matching must not run when task is absent")

			return false, nil
		},

		// inspectConfig
		func(options Options) (bool, error) {
			t.Fatal("configuration inspection must not run when task is absent")

			return false, nil
		},

		// inspectFirewall
		func(
			ctx context.Context,
			options Options,
		) (bool, error) {
			t.Fatal("firewall inspection must not run when task is absent")

			return false, nil
		},

		// checkHealth
		func(ctx context.Context) error {
			t.Fatal("health check must not run when task is absent")

			return nil
		},
	)
	if err != nil {
		t.Fatalf("inspectInstallation() error = %v", err)
	}

	if state != (installationState{}) {
		t.Fatalf(
			"installation state = %+v, want absent state",
			state,
		)
	}
}

func TestInspectInstallationReportsHealthyDriftedTask(t *testing.T) {
	state, err := inspectInstallation(
		context.Background(),
		DefaultOptions(),

		// inspectTask
		func(
			ctx context.Context,
			taskName string,
		) (bool, error) {
			return true, nil
		},

		// inspectTaskEnabled
		func(
			ctx context.Context,
			taskName string,
		) (bool, error) {
			return true, nil
		},

		// matchTask
		func(
			ctx context.Context,
			options Options,
		) (bool, error) {
			return false, nil
		},

		// inspectConfig
		func(options Options) (bool, error) {
			return true, nil
		},

		// inspectFirewall
		func(
			ctx context.Context,
			options Options,
		) (bool, error) {
			return true, nil
		},

		// checkHealth
		func(ctx context.Context) error {
			return nil
		},
	)
	if err != nil {
		t.Fatalf("inspectInstallation() error = %v", err)
	}

	if !state.TaskExists || !state.TaskEnabled || !state.Healthy {
		t.Fatalf("installation state = %+v, want healthy task", state)
	}
	if state.TaskMatches {
		t.Fatal("TaskMatches = true, want false for task definition drift")
	}
}

func TestInspectInstallationSkipsOtherChecksWhenTaskIsDisabled(
	t *testing.T,
) {
	otherCheckCalled := false

	state, err := inspectInstallation(
		context.Background(),
		DefaultOptions(),

		// inspectTask
		func(
			ctx context.Context,
			taskName string,
		) (bool, error) {
			return true, nil
		},

		// inspectTaskEnabled
		func(
			ctx context.Context,
			taskName string,
		) (bool, error) {
			return false, nil
		},

		// matchTask
		func(
			ctx context.Context,
			options Options,
		) (bool, error) {
			otherCheckCalled = true

			return true, nil
		},

		// inspectConfig
		func(options Options) (bool, error) {
			otherCheckCalled = true

			return true, nil
		},

		// inspectFirewall
		func(
			ctx context.Context,
			options Options,
		) (bool, error) {
			otherCheckCalled = true

			return true, nil
		},

		// checkHealth
		func(ctx context.Context) error {
			otherCheckCalled = true

			return nil
		},
	)
	if err != nil {
		t.Fatalf("inspectInstallation() error = %v", err)
	}

	want := installationState{
		TaskExists: true,
	}

	if state != want {
		t.Fatalf("installation state = %+v, want %+v", state, want)
	}
	if otherCheckCalled {
		t.Fatal("other LHM checks ran for a disabled task")
	}
}
