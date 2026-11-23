// Package application defines Runtime Collector activation workflows and their
// collaboration contracts. It accepts revision-bound candidate requests and
// returns durable admission results; SQLite persistence, configuration
// materialization, Collector process control, and platform recovery remain
// behind injected adapters and ports.
package application
