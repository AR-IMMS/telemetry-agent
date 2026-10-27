// Package agentinstallation installs and removes the Agent binary and its OS
// service using platform-specific adapters.
//
// It coordinates the durable installation layout, Collector bootstrap,
// ownership persistence, and service registration. It does not own ongoing
// Collector supervision or remove resources that the Agent does not own.
package agentinstallation
