package windowsexporter

import dependencies "github.com/ar-imms/telemetry-agent/internal/runtime/dependencies"

// Definition is the shared Runtime managed dependency definition.
type Definition = dependencies.Definition

// Enabler is the shared Runtime dependency enable contract.
type Enabler = dependencies.Enabler

// InstallResult records the shared dependency installation result.
type InstallResult = dependencies.InstallResult

// Inspector is the shared Runtime dependency inspection contract.
type Inspector = dependencies.Inspector

// Inspection contains the shared dependency inspection result.
type Inspection = dependencies.Inspection

// Status is the shared Runtime dependency lifecycle status.
type Status = dependencies.Status

// Teardown is the shared Runtime dependency removal contract.
type Teardown = dependencies.Teardown

// ArtifactDownloader acquires a pinned Windows Exporter artifact.
type ArtifactDownloader = dependencies.ArtifactDownloader

// WindowsExporterInstaller installs the Windows Exporter dependency.
type WindowsExporterInstaller = dependencies.Installer
