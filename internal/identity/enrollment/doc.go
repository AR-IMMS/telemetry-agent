// Package enrollment owns Agent-side enrollment policy and contracts.
//
// It validates enrollment inputs and Core results and coordinates durable
// attempts through narrow ports. HTTP, credential storage, SQLite, Collector
// lifecycle, and Core configuration ownership are implemented elsewhere.
package enrollment
