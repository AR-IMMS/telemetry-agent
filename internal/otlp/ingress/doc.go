// Package ingress authenticates OTLP/HTTP JSON at the public Gateway edge.
// It receives Agent telemetry, replaces client identity with Fleet-verified
// agent_id, and forwards sanitized OTLP to an internal Collector Gateway.
// Fleet owns credentials; this package does not issue or cache them.
package ingress
