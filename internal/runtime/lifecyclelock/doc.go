// Package lifecyclelock owns the lock-file naming policy used to serialize
// Collector configuration and managed-resource side effects across processes.
// It delegates the operating-system lock primitive to platform/filelock and
// does not own state persistence or SQLite transactions.
package lifecyclelock
