package nodeexporter

// Artifact identifies one checksum-pinned Node Exporter release archive.
type Artifact struct {
	// Version is the upstream Node Exporter release version.
	Version string

	// ArchiveName is the local file name used during staging.
	ArchiveName string

	// ArchiveFormat describes the expected archive container.
	ArchiveFormat string

	// URL is the immutable upstream release-asset location.
	URL string

	// SHA256 is the expected lower-case archive checksum.
	SHA256 string

	// BinaryPath is the expected relative binary path inside the archive.
	BinaryPath string
}

// PinnedArtifact returns the only Node Exporter archive this Agent release
// is allowed to install.
func PinnedArtifact() Artifact {
	return Artifact{
		Version:       "1.12.1",
		ArchiveName:   "node_exporter-1.12.1.linux-amd64.tar.gz",
		ArchiveFormat: "tar.gz",
		URL: "https://github.com/prometheus/node_exporter/releases/download/" +
			"v1.12.1/node_exporter-1.12.1.linux-amd64.tar.gz",
		SHA256: "b51d8a76aa2a9156a55d501aca6276fae09e262259a5e4e831d2c2222f084e63",
		BinaryPath: "node_exporter-1.12.1.linux-amd64/" +
			"node_exporter",
	}
}
