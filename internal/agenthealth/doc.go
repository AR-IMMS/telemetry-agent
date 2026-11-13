// Package agenthealth evaluates transient Agent, Supervisor, and Collector
// observations into a local evidence assessment for status and diagnostic
// consumers. It does not persist desired state, own recovery, or prove that
// telemetry was scraped, exported, or received by a backend.
package agenthealth
