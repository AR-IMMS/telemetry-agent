// Package sqlite adapts Inventory snapshots and provider status to the shared SQLite database.
//
// It owns Inventory migration definitions and preserves snapshot, status, and revision
// updates in their existing transactions. The composition boundary registers those
// migrations in the global version order; this package does not own the database
// lifecycle.
package sqlite
