// Package handlers implements the explicitly cataloged, read-only Operations
// use cases. It consumes Health, Docker inspection, and managed-dependency
// ports and returns allowlisted evidence; it does not own persistence,
// transport, authorization, or resource discovery.
package handlers
