package inventory

import monitoringcontract "github.com/ar-imms/telemetry-agent/internal/contracts/monitoring"

// Docker label keys are the allowlisted metadata contract observed from
// containers. Inventory preserves them as evidence; Monitoring interprets
// their values and decides whether a profile applies.
const (
	// DockerLabelMonitoringEnabled records explicit opt-in to Agent monitoring.
	DockerLabelMonitoringEnabled = monitoringcontract.DockerLabelEnabled
	// DockerLabelMonitoringProfile records the requested reviewed profile ID.
	DockerLabelMonitoringProfile = monitoringcontract.DockerLabelProfile
	// DockerLabelMonitoringPort records an optional profile endpoint override.
	DockerLabelMonitoringPort = monitoringcontract.DockerLabelPort
	// DockerLabelWorkloadID records an optional stable logical workload identity.
	DockerLabelWorkloadID = monitoringcontract.DockerLabelWorkloadID
)
