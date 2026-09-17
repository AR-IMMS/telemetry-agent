// Package nodeexporter installs and reconciles the Agent-managed Node Exporter.
//
// It stages and verifies the pinned Linux artifact, publishes the executable,
// renders the Agent-owned systemd unit, supervises service startup and restart,
// and verifies the local metrics endpoint. Existing healthy services are reused
// when their managed unit matches the current rendering; drifted units are
// updated without reinstalling the archive.
//
// Nodeexporter does not collect or normalize telemetry, render OpenTelemetry
// Collector configuration, or communicate with telemetry gateways.
package nodeexporter
