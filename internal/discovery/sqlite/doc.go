// Package sqlite implements Discovery inventory and proposal repositories on
// the Agent's shared SQLite database. It persists Discovery-owned records but
// intentionally leaves matching, proposal transitions, and approval workflows
// to the Discovery and Configuration application layers.
package sqlite
