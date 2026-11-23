// Package sqlite implements Configuration's SQLite repositories for desired
// Integrations, Templates, proposals, materialization, approval, and
// reconciliation evidence. It uses the shared Agent database and leaves
// workflow decisions to the Configuration application layer. Runtime journal
// evidence is read through a transaction-scoped adapter; Configuration table
// writes and any related multi-table operation remain atomic.
package sqlite
