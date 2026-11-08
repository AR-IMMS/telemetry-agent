// Package reconciliation coordinates desired telemetry configuration toward
// the existing Collector lifecycle. It owns retries and convergence evidence,
// but not desired state, compilation, activation, health, or delivery.
package reconciliation
