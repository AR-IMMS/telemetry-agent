package inventory

// Docker label keys are the allowlisted metadata contract observed from
// containers. Inventory preserves them as evidence; Monitoring interprets
// their values and decides whether a profile applies.
const (
	// DockerLabelMonitoringEnabled records explicit opt-in to Agent monitoring.
	DockerLabelMonitoringEnabled = "ar-imms.monitoring.enabled"
	// DockerLabelMonitoringProfile records the requested reviewed profile ID.
	DockerLabelMonitoringProfile = "ar-imms.monitoring.profile"
	// DockerLabelMonitoringPort records an optional profile endpoint override.
	DockerLabelMonitoringPort = "ar-imms.monitoring.port"
	// DockerLabelWorkloadID records an optional stable logical workload identity.
	DockerLabelWorkloadID = "ar-imms.workload.id"
)
