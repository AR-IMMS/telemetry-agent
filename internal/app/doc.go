// Package app owns Agent Service composition lifecycle and its long-running
// control-loop coordination. It accepts runtime adapters from the executable,
// runs discovery/materialization/reconciliation under one context, and
// delegates domain policy, persistence, CLI behavior, and OS service control to
// their owning packages.
package app
