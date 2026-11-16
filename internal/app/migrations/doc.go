// Package migrations assembles context-owned SQLite migrations at the
// Application composition boundary. It preserves the global version order
// while delegating SQL and transaction semantics to each owner; it does not
// open databases or own context tables.
package migrations
