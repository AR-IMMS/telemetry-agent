// Package integration is a transitional compatibility adapter for the former
// Integration API. Desired-state aliases delegate to Configuration, and shipped
// definitions, registry/resolution, matching contracts, and the matcher delegate
// to Monitoring. Template models and template-match orchestration remain during
// their owner migration. The package does not persist state or control processes.
// Remove it after production callers use Configuration and Monitoring owner
// packages directly and persisted-data, CLI, and template-matching parity tests pass.
package integration
