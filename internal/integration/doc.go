// Package integration temporarily preserves the pre-consolidation Integration
// API while Monitoring definitions and Configuration desired-state types migrate
// to their owning capabilities. Desired-state aliases and validation delegate
// to Configuration; profile definitions and matching remain here until they
// move under internal/monitoring. Retire this package when all production
// consumers use owner packages directly and persisted/API compatibility tests
// no longer require the bridge. It does not persist state or control processes.
package integration
