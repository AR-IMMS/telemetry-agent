// Package agentstate persists the Agent's machine-local desired state.
//
// It stores Collector runtime context, managed dependency state, configuration
// generations, pending teardown requests, and resources owned by each
// dependency. SQLiteStore persists operational state in WAL mode with short
// transactions and imports legacy JSON state on first access. FileStore remains
// available for JSON compatibility and isolated tests; DefaultPath selects the
// platform's machine-wide SQLite database location.
//
// The package records lifecycle intent and ownership but does not render or
// start Collector configurations, execute dependency installation or teardown,
// or implement platform service management.
package agentstate
