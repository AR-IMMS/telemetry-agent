// Package collectorconfig renders and validates layered OpenTelemetry Collector configuration.
//
// It merges ordered YAML layers with explicit semantics, applies supplied value
// substitutions, validates the merged document, and returns rendered YAML.
//
// It does not own reconciliation policy, install or supervise the Collector, or
// communicate with Core.
package collectorconfig
