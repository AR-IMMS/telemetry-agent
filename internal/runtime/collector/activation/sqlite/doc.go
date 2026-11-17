// Package sqlite owns Runtime Collector activation schema migrations and the
// durable history format for candidate activation evidence, including Gateway
// runtime seeding. It consumes the shared SQLite transaction and does not own
// database paths or Configuration snapshot validation.
package sqlite
