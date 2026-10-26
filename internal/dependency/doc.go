// Package dependency defines built-in telemetry dependency integrations and
// coordinates their install, inspection, enablement, and teardown operations.
//
// Platform adapters perform OS-specific work, while the Agent lifecycle owns
// Collector configuration activation. This package does not own Collector
// process supervision or the shared operational-state schema.
package dependency
