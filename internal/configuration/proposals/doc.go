// Package proposals owns the monitoring-proposal lifecycle and approval
// contract within Configuration. It validates Monitoring recommendations,
// rechecks fresh Inventory evidence through a port, and requests an atomic
// proposal/desired-state commit; it does not discover resources or activate
// the Collector.
package proposals
