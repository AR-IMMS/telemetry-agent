// Package agentstate persists the Agent's machine-local desired state.
//
// It stores Collector runtime context, managed dependency state, configuration
// generations, pending teardown requests, and resources owned by each
// dependency. FileStore reads and atomically replaces the JSON state while an
// operating-system lock serializes concurrent updates; DefaultPath selects the
// platform's machine-wide state location.
//
// The package records lifecycle intent and ownership but does not render or
// start Collector configurations, execute dependency installation or teardown,
// or implement platform service management.
package agentstate
