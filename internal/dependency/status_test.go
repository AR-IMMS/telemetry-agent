package dependency

import "testing"

func TestInspectionStatusReportsDisabledWithoutHealth(
	t *testing.T,
) {
	status := Inspection{
		Enabled: false,
	}.Status()

	if status.Availability != AvailabilityDisabled {
		t.Fatalf(
			"availability = %q, want %q",
			status.Availability,
			AvailabilityDisabled,
		)
	}

	if status.Health != HealthUnknown {
		t.Fatalf(
			"health = %q, want %q",
			status.Health,
			HealthUnknown,
		)
	}
}

func TestInspectionStatusReportsEnabledHealthy(
	t *testing.T,
) {
	status := Inspection{
		Enabled: true,
		Healthy: true,
	}.Status()

	if status.Availability != AvailabilityEnabled {
		t.Fatalf(
			"availability = %q, want %q",
			status.Availability,
			AvailabilityEnabled,
		)
	}

	if status.Health != HealthHealthy {
		t.Fatalf(
			"health = %q, want %q",
			status.Health,
			HealthHealthy,
		)
	}
}

func TestInspectionStatusReportsEnabledUnhealthy(
	t *testing.T,
) {
	status := Inspection{
		Enabled: true,
		Healthy: false,
	}.Status()

	if status.Availability != AvailabilityEnabled {
		t.Fatalf(
			"availability = %q, want %q",
			status.Availability,
			AvailabilityEnabled,
		)
	}

	if status.Health != HealthUnhealthy {
		t.Fatalf(
			"health = %q, want %q",
			status.Health,
			HealthUnhealthy,
		)
	}
}

func TestInspectionStatusReportsEnabledDrifted(
	t *testing.T,
) {
	status := Inspection{
		Enabled: true,
		Healthy: true,
		Drifted: true,
	}.Status()

	if status.Availability != AvailabilityEnabled {
		t.Fatalf(
			"availability = %q, want %q",
			status.Availability,
			AvailabilityEnabled,
		)
	}

	if status.Health != HealthDrifted {
		t.Fatalf(
			"health = %q, want %q",
			status.Health,
			HealthDrifted,
		)
	}
}
