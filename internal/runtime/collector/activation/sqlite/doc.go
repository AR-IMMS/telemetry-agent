// Package sqlite owns Runtime Collector activation schema migrations and the
// durable history format for candidate activation evidence. It consumes the
// shared SQLite transaction and does not own database paths or other contexts' tables.
package sqlite
