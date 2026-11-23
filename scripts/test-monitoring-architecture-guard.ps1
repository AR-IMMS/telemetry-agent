$ErrorActionPreference = 'Stop'
$repoRoot = [System.IO.Path]::GetFullPath((Split-Path -Parent $PSScriptRoot))
$legacyPackageDirectory = Join-Path $repoRoot 'internal/integration'
$legacyPackageFixture = Join-Path $repoRoot 'internal/integration/doc.go'
$importFixture = Join-Path $repoRoot 'internal/monitoring/profiles/definitions/redis/architecture_guard_fixture.go'
$domainAdapterFixture = Join-Path $repoRoot 'internal/monitoring/matching/domain/architecture_guard_fixture.go'
$applicationAdapterFixture = Join-Path $repoRoot 'internal/configuration/desiredstate/application/architecture_guard_fixture.go'
$sizeFixture = Join-Path $repoRoot 'internal/cli/commands/inventory/architecture_guard_size_fixture.go'
$guardPath = Join-Path $PSScriptRoot 'check-architecture.ps1'

if ((Test-Path -LiteralPath $legacyPackageDirectory) -or (Test-Path -LiteralPath $importFixture) -or
    (Test-Path -LiteralPath $domainAdapterFixture) -or (Test-Path -LiteralPath $applicationAdapterFixture) -or
    (Test-Path -LiteralPath $sizeFixture)) {
    throw 'Refusing to overwrite an existing Monitoring architecture fixture.'
}

$modulePath = (& go list -m).Trim()
if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($modulePath)) {
    throw 'Unable to resolve the Go module path.'
}

try {
    New-Item -ItemType Directory -Path $legacyPackageDirectory | Out-Null
    @"
// Package integration is a temporary fixture for retired-path enforcement.
package integration
"@ | Set-Content -LiteralPath $legacyPackageFixture -Encoding utf8

    @"
package redis

import _ "$modulePath/internal/integration"
"@ | Set-Content -LiteralPath $importFixture -Encoding utf8

    $output = & pwsh -NoProfile -File $guardPath 2>&1 | Out-String
    $guardExit = $LASTEXITCODE
    if ($guardExit -eq 0 -or $output -notmatch 'retired internal/integration package') {
        throw "The architecture guard did not reject the retired Monitoring path. Output:`n$output"
    }
    Write-Output 'Negative fixture rejected: Monitoring cannot recreate or import internal/integration.'
    Remove-Item -LiteralPath $importFixture
    Remove-Item -LiteralPath $legacyPackageFixture
    Remove-Item -LiteralPath $legacyPackageDirectory

    @"
package domain

import _ "$modulePath/internal/platform/sqlite"
"@ | Set-Content -LiteralPath $domainAdapterFixture -Encoding utf8
    $output = & pwsh -NoProfile -File $guardPath 2>&1 | Out-String
    $guardExit = $LASTEXITCODE
    if ($guardExit -eq 0 -or $output -notmatch 'domain package imports adapter') {
        throw "The architecture guard did not reject infrastructure imports from a Monitoring domain package. Output:`n$output"
    }
    Write-Output 'Negative fixture rejected: Monitoring domain packages cannot import SQLite adapters.'

    @"
package application

import _ "$modulePath/internal/platform/sqlite"
"@ | Set-Content -LiteralPath $applicationAdapterFixture -Encoding utf8
    $output = & pwsh -NoProfile -File $guardPath 2>&1 | Out-String
    $guardExit = $LASTEXITCODE
    if ($guardExit -eq 0 -or $output -notmatch 'application package imports adapter') {
        throw "The architecture guard did not reject infrastructure imports from a Configuration application package. Output:`n$output"
    }
    Write-Output 'Negative fixture rejected: Configuration application packages cannot import platform SQLite.'
    Remove-Item -LiteralPath $domainAdapterFixture
    Remove-Item -LiteralPath $applicationAdapterFixture

    $sizeLines = @('package inventory') + (2..451 | ForEach-Object { '// temporary architecture guard size fixture' })
    $sizeLines | Set-Content -LiteralPath $sizeFixture -Encoding utf8
    $output = & pwsh -NoProfile -File $guardPath 2>&1 | Out-String
    $guardExit = $LASTEXITCODE
    if ($guardExit -eq 0 -or $output -notmatch 'architecture_guard_size_fixture.go has 451 lines \(maximum 450\)') {
        throw "The architecture guard did not report an oversized production source file. Output:`n$output"
    }
    Write-Output 'Negative fixture rejected: production source files cannot exceed 450 lines.'
}
finally {
    if (Test-Path -LiteralPath $importFixture) {
        Remove-Item -LiteralPath $importFixture
    }
    if (Test-Path -LiteralPath $domainAdapterFixture) {
        Remove-Item -LiteralPath $domainAdapterFixture
    }
    if (Test-Path -LiteralPath $applicationAdapterFixture) {
        Remove-Item -LiteralPath $applicationAdapterFixture
    }
    if (Test-Path -LiteralPath $sizeFixture) {
        Remove-Item -LiteralPath $sizeFixture
    }
    if (Test-Path -LiteralPath $legacyPackageFixture) {
        Remove-Item -LiteralPath $legacyPackageFixture
    }
    if (Test-Path -LiteralPath $legacyPackageDirectory) {
        Remove-Item -LiteralPath $legacyPackageDirectory
    }
}

& pwsh -NoProfile -File $guardPath
if ($LASTEXITCODE -ne 0) {
    throw 'The architecture guard failed after the negative fixture was removed.'
}
Write-Output 'Monitoring architecture guard positive check passed.'
