package domain

import inventoryctx "github.com/ar-imms/telemetry-agent/internal/inventory"

// EndpointStrategiesFor returns the reviewed platform endpoint rules for a profile.
// The rules describe candidates; they do not assert that a discovered endpoint is reachable.
func EndpointStrategiesFor(profileID, receiverType string, targetPort uint16, targetPortLabel string) []EndpointStrategy {
	alternateID := "alternate_port"
	containerID := "container_endpoint"
	if profileID != "redis" {
		alternateID = profileID + "_alternate_port"
		containerID = profileID + "_container_endpoint"
	}
	endpointFor := func(scope string) string {
		switch receiverType {
		case "redis", "postgresql":
			if scope == "host" {
				return "127.0.0.1:`alternate_port`"
			}
			return "`host`:`port`"
		default:
			if scope == "host" {
				return "127.0.0.1:`alternate_port`"
			}
			return "`joinHostPort(host, port)`"
		}
	}
	return []EndpointStrategy{
		{ID: alternateID, OS: "windows", When: "alternate_port > 0", Endpoint: endpointFor("host"), Scope: inventoryctx.EndpointHost, Provenance: "published_port", TargetPort: targetPort, TargetPortLabel: targetPortLabel, PrivatePortVariable: "port", Priority: 100},
		{ID: alternateID, OS: "linux", When: "alternate_port > 0", Endpoint: "`host`:`port`", Scope: inventoryctx.EndpointHost, Provenance: "published_port", TargetPort: targetPort, TargetPortLabel: targetPortLabel, PrivatePortVariable: "alternate_port", Priority: 100},
		{ID: containerID, OS: "linux", When: "alternate_port == 0 and host != \"\"", Endpoint: endpointFor("container"), Scope: inventoryctx.EndpointContainer, Provenance: "container_network", TargetPort: targetPort, TargetPortLabel: targetPortLabel, PrivatePortVariable: "port", Priority: 50},
	}
}
