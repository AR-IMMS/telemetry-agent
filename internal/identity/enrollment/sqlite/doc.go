// Package sqlite adapts enrollment attempt persistence to SQLite.
//
// It stores secret-free attempt metadata and opaque credential references in
// the shared operational database. The caller supplies database opening and
// in-process write serialization; this package does not own Core communication,
// credential values, retry policy, or credential cleanup side effects.
package sqlite
