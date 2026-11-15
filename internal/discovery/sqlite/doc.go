// Package sqlite stores the transitional proposal repository in the shared Agent SQLite database.
//
// Proposal persistence and approval move together to Configuration in PR-09.5c.
// Migration registration remains centralized and preserves the existing transaction.
package sqlite
