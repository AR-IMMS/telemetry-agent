// Package librehardwaremonitor manages Libre Hardware Monitor on Windows.
//
// The package installs one checksum-pinned upstream release into the
// Agent-owned installation directory and configures its Prometheus endpoint on
// TCP port 9190.
//
// Libre Hardware Monitor runs as a LocalSystem scheduled task at system
// startup. Its HTTP listener may bind beyond loopback due to an upstream
// limitation, so the Agent owns a Windows Firewall rule that blocks inbound
// TCP traffic to the metrics port. The local OTel Collector is the only
// intended scraper.
//
// Installation requires an Administrator terminal. A healthy matching
// installation is reused; managed configuration, firewall, or task drift is
// repaired without downloading the archive again. An unhealthy existing task
// is never overwritten automatically.
package librehardwaremonitor
