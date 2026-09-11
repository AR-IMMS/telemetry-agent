// Package config renders and validates layered OpenTelemetry Collector configuration.
//
// It merges ordered YAML layers with explicit semantics, applies supplied value
// substitutions, validates the merged document, and returns rendered YAML.
//
// Config does not install, start, supervise, or communicate with the Collector.
package config
