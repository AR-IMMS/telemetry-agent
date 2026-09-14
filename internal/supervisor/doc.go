// Package supervisor starts, monitors, and stops the OpenTelemetry Collector.
//
// It launches one verified Collector process, injects runtime endpoint
// configuration, waits for health readiness, and coordinates graceful or forced
// shutdown. It does not install binaries, render configuration, collect
// telemetry, or own gateway business logic.
package supervisor
