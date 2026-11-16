package windowsexporter

import dependencies "github.com/ar-imms/telemetry-agent/internal/runtime/dependencies"

// These aliases expose the shared lifecycle contracts consumed by the
// Windows Exporter adapter without defining a second dependency model.
type Definition = dependencies.Definition
type Enabler = dependencies.Enabler
type InstallResult = dependencies.InstallResult
type Inspector = dependencies.Inspector
type Inspection = dependencies.Inspection
type Status = dependencies.Status
type Teardown = dependencies.Teardown
type ArtifactDownloader = dependencies.ArtifactDownloader
type WindowsExporterInstaller = dependencies.Installer
