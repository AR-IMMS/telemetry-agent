// Package sqlite implements Configuration's SQLite repositories for desired
// Integrations, Templates, materialization, and approval. It uses the shared
// Agent database and leaves validation and projection decisions to the
// Configuration application layer; multi-table operations remain atomic.
package sqlite
