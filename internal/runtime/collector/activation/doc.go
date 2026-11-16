// Package agentlifecycle coordinates persisted dependency state with the
// running OpenTelemetry Collector.
//
// It renders, validates, and activates Collector configuration for the current
// desired state; records activated and applied configuration generations;
// supervises Collector restarts when an activated generation changes; and
// invokes pending dependency teardowns only after the Collector has applied the
// generation that removes the dependency receiver.
//
// The package delegates configuration rendering and native validation to
// config/bootstrap, process supervision to supervisor, and host-resource
// changes to an injected teardown function. It does not own persisted state's
// schema, dependency installation, platform service management, or the
// platform-specific teardown implementations themselves.
package agentlifecycle
