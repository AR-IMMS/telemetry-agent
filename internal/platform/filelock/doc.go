// Package filelock provides an exclusive cross-process lock over a named file.
// It returns a handle that keeps the lock until Close and does not own the
// calling capability's lock-path policy or lifecycle decisions.
package filelock
