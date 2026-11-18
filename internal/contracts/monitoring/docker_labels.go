package monitoring

// Docker label keys opt a workload into reviewed Agent monitoring profiles.
const (
	// DockerLabelEnabled explicitly opts one workload into monitoring.
	DockerLabelEnabled = "ar-imms.monitoring.enabled"
	// DockerLabelProfile selects a reviewed profile ID.
	DockerLabelProfile = "ar-imms.monitoring.profile"
	// DockerLabelPort optionally overrides a profile's default TCP port.
	DockerLabelPort = "ar-imms.monitoring.port"
	// DockerLabelWorkloadID supplies an optional stable logical workload identity.
	DockerLabelWorkloadID = "ar-imms.workload.id"
)
