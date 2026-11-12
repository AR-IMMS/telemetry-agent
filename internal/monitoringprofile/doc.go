// Package monitoringprofile owns the small, validated registry of Agent
// monitoring profiles and projects supported profiles into Collector rules.
// It consumes normalized provider attributes and returns configuration intent;
// it does not discover resources, probe endpoints, or supervise the Collector.
package monitoringprofile
