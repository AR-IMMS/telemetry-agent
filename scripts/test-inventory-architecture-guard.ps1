$ErrorActionPreference = 'Stop'
$repoRoot = [System.IO.Path]::GetFullPath((Split-Path -Parent $PSScriptRoot))
$fixturePath = Join-Path $repoRoot 'internal/inventory/docker/architecture_guard_fixture.go'
$facadePackageFixturePath = Join-Path $repoRoot 'internal/inventory/doc.go'
$guardPath = Join-Path $PSScriptRoot 'check-architecture.ps1'

if ((Test-Path -LiteralPath $fixturePath) -or (Test-Path -LiteralPath $facadePackageFixturePath)) {
    throw 'Refusing to overwrite an existing Inventory architecture fixture.'
}

$modulePath = (& go list -m).Trim()
if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($modulePath)) {
    throw 'Unable to resolve the Go module path.'
}

try {
    @"
// Package inventory is a temporary package used to exercise the architecture guard.
package inventory
"@ | Set-Content -LiteralPath $facadePackageFixturePath -Encoding utf8

    @"
package docker

import _ "$modulePath/internal/inventory"
"@ | Set-Content -LiteralPath $fixturePath -Encoding utf8

    $output = & pwsh -NoProfile -File $guardPath 2>&1 | Out-String
    $guardExit = $LASTEXITCODE
    if ($guardExit -eq 0 -or $output -notmatch 'Production package must use Inventory observation owner contracts instead of the root compatibility facade') {
        throw "The architecture guard did not reject the compatibility-facade fixture. Output:`n$output"
    }
    Write-Output 'Negative fixture rejected: Inventory adapters cannot import the root compatibility facade.'
}
finally {
    if (Test-Path -LiteralPath $fixturePath) {
        Remove-Item -LiteralPath $fixturePath
    }
    if (Test-Path -LiteralPath $facadePackageFixturePath) {
        Remove-Item -LiteralPath $facadePackageFixturePath
    }
}

& pwsh -NoProfile -File $guardPath
if ($LASTEXITCODE -ne 0) {
    throw 'The architecture guard failed after the negative fixture was removed.'
}
Write-Output 'Inventory architecture guard positive fixture passed.'
