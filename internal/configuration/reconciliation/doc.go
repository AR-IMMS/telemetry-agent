// Package reconciliation coordinates desired telemetry configuration toward
// the existing Collector lifecycle. It owns retries, convergence evidence, and
// consumer-side baseline/application ports, but not desired state, compilation,
// activation, health, or delivery. Runtime adapters implement its ports.
package reconciliation
