// Package sqlite persists Configuration-owned Core Gateway snapshots and
// revisions in SQLite. It owns schema versions 3 and 4 and their atomic
// transactions; Agent generation and activation evidence uses transaction-
// scoped callbacks so related writes remain atomic.
package sqlite
