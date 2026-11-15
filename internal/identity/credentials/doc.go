// Package credentials stores Agent secrets outside SQLite using
// service-scoped OS protections: machine DPAPI and restricted ACLs on Windows,
// and service-owned files with restrictive permissions on Linux.
//
// Values are stored by opaque reference and are never exposed in errors. The
// package also materializes the protected Supervisor token/config files; Runtime
// chooses their contents and lifecycle, while this package enforces file access.
package credentials
