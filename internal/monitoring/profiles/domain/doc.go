// Package domain owns validated monitoring profile definitions and the policy
// used to select profiles from provider observations. It returns immutable
// registry snapshots and endpoint strategies; it does not load files, inspect
// Collector logs, query runtime capabilities, or render Collector config.
package domain
