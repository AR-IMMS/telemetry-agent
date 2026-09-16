package dependency

// PinnedWindowsExporterArtifact returns the only MSI this Agent release can install.
func PinnedWindowsExporterArtifact() WindowsExporterArtifact {
	return WindowsExporterArtifact{
		Version:  "0.31.8",
		FileName: "windows_exporter-0.31.8-amd64.msi",
		URL: "https://github.com/prometheus-community/windows_exporter/" +
			"releases/download/v0.31.8/" +
			"windows_exporter-0.31.8-amd64.msi",
		SHA256: "0aadce6afb20182b678bfca9e8f2e8464ef48c469" +
			"b28b4cf02e99d82158f5d40",
	}
}
