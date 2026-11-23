// Package dependency owns the compatibility command group and presentation for
// existing Runtime dependency catalog, status, and pending-teardown operations.
//
// It receives Runtime results through narrow callbacks, writes legacy text and
// exit-code contracts, and forwards the existing command paths. It does not own
// dependency policy, installation, persistence, or platform lifecycle behavior.
package dependency
