// Package activationdomain owns persisted Runtime Collector activation
// identities and recovery intent. Agent-state storage serializes these values;
// this package does not own SQLite access or Collector process lifecycle.
package activationdomain
