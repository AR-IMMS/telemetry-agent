// Package bootstrap provisions and validates the pinned OpenTelemetry Collector.
//
// It selects a supported platform artifact, verifies and safely extracts the
// downloaded binary, and returns the installed binary and configuration result.
//
// Bootstrap does not own service registration, Collector supervision, telemetry
// collection, or communication with the gateway.
package bootstrap
