// Package sqlite adapts Inventory snapshots and provider status to the shared SQLite database.
//
// It preserves snapshot, status, and revision updates in their existing atomic
// transactions. Schema migration ownership remains in the central coordinator.
package sqlite
