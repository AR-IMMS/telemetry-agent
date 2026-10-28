// Package identity collects host attributes and assigns stable Agent
// installation identity.
//
// It returns normalized platform information and owns the value and policy for
// installation UUIDs. Installation UUID persistence is provided through a
// narrow repository interface implemented by the Agent state package.
//
// Identity does not own SQLite, OS service management, workload or container
// identities, software installation, Collector lifecycle, or Core connectivity.
package identity
