// Package sqlite provides the Agent's configured SQLite connection boundary.
// It accepts a database path and caller-owned initializer, returns one
// configured SQL handle, and intentionally owns no domain tables, migration
// SQL, or transaction semantics.
package sqlite
