package contracts

import profile "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/domain"

// ResolvedContribution pairs one stable desired-state identifier with the
// Monitoring-owned, runtime-neutral contribution that Runtime compiles. The
// desired-state model and its lifecycle policy remain owned by Configuration.
type ResolvedContribution struct {
	InstanceID   string
	Contribution profile.Contribution
}

// DockerMonitoringProfile is the Runtime-owned input needed to render one
// validated Docker monitoring intent into Collector configuration.
type DockerMonitoringProfile struct {
	ID                   string
	Provider             string
	MatchLabels          map[string]string
	RequiredCapabilities []string
	ReceiverType         string
	CollectionInterval   string
	EndpointStrategies   []DockerEndpointStrategy
	Settings             DockerReceiverSettings
}

// DockerEndpointStrategy describes a provider endpoint choice without exposing
// Inventory or Monitoring implementation types to Runtime.
type DockerEndpointStrategy struct {
	ID                  string
	OS                  string
	When                string
	Endpoint            string
	Scope               string
	Provenance          string
	TargetPort          uint16
	TargetPortLabel     string
	PrivatePortVariable string
	Priority            int
}

// DockerReceiverSettings contains the validated settings supported by native
// Collector receiver projections.
type DockerReceiverSettings struct {
	MetricsPath string
	UsernameEnv string
	PasswordEnv string
	TLSCAFile   string
}
