// Package installation owns the stable identifier for one Agent installation
// and its initialize-once identity policy.
//
// It produces a validated installation ID through a repository port. SQLite
// persistence is implemented by the sibling Identity adapter, not here.
//
// Installation identity does not own Core enrollment, host discovery, secret
// storage, or operational Agent state unrelated to the installation record.
package installation
