// Package application coordinates enrollment use cases through repository,
// credential-store, and Core-client ports. It preserves durable retry and
// credential ordering and does not implement SQLite, protected storage, HTTP,
// or Collector lifecycle behavior.
package application
