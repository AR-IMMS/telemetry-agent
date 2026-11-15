// Package identity collects operating-system and host facts used by the Agent.
//
// It returns normalized platform, machine, and release metadata. Installation
// identity and enrollment are owned by sibling Identity capabilities.
//
// Identity host facts do not own persistence, workload inventory, OS service
// management, software installation, Collector lifecycle, or Core connectivity.
package identity
