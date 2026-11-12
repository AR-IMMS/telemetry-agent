// Package sqlite adapts installation identity persistence to SQLite.
//
// It accepts a database opener and returns the committed installation ID; the
// caller owns schema migration and database lifecycle. This package does not
// own identity generation policy, other Agent state, or secret storage.
package sqlite
