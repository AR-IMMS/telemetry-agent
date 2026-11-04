// Package credentialstore stores Agent secrets outside SQLite using
// service-scoped OS protections: machine DPAPI and restricted ACLs on Windows,
// and service-owned files with restrictive permissions on Linux.
//
// Values are stored outside SQLite and are never exposed in errors. This
// package does not own enrollment policy, credential lifetimes, or Core auth.
package credentialstore
