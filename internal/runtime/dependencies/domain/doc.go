// Package dependencydomain owns managed dependency state and teardown intent.
// Its models are persisted by the Agent-state adapter and consumed by Runtime
// dependency lifecycle and Collector activation; it does not own persistence
// or platform-specific execution.
package dependencydomain
