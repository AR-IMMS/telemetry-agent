// Package operations validates, executes, audits, and reports Core-authorized
// read-only operations for this Agent installation. It owns operation
// contracts and execution coordination, while handlers consume narrow
// read-only context ports; it does not own business authorization, transport,
// Docker SDK details, or SQLite implementation.
package operations
