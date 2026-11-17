// Package domain owns provider-neutral inventory identity, observations,
// normalization, freshness, and absence reconciliation invariants. It accepts
// discovery evidence and returns validated snapshots or inventory records; it
// does not orchestrate providers, persist records, or assess endpoint reachability.
package domain
