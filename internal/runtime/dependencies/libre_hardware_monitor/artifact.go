package librehardwaremonitor

// Artifact identifies one checksum-pinned upstream LHM ZIP release.
type Artifact struct {
	// Version is the upstream Libre Hardware Monitor release version.
	Version string

	// FileName is the expected ZIP name in the private staging directory.
	FileName string

	// URL is the immutable GitHub release-asset location.
	URL string

	// SHA256 is the expected lower-case SHA-256 checksum of the ZIP bytes.
	SHA256 string
}

// PinnedArtifact returns the only LHM release this Agent version installs.
func PinnedArtifact() Artifact {
	return Artifact{
		Version:  "0.9.6",
		FileName: "LibreHardwareMonitor.zip",
		URL: "https://github.com/LibreHardwareMonitor/" +
			"LibreHardwareMonitor/releases/download/v0.9.6/" +
			"LibreHardwareMonitor.zip",
		SHA256: "086d9f1b5a99e643edc2cfaaac160516" +
			"85b551e4c5ac0b32a57c58c0e529c001",
	}
}
