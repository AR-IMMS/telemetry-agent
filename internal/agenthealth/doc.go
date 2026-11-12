// Package agenthealth evaluates transient Agent and Collector observations into
// a local readiness snapshot for status and diagnostic consumers. It does not
// persist desired state or prove that telemetry was scraped, exported, or
// received by a backend.
package agenthealth
