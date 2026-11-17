$ErrorActionPreference = 'Stop'
$repoRoot = [System.IO.Path]::GetFullPath((Split-Path -Parent $PSScriptRoot))
$fixturePath = Join-Path $repoRoot 'internal/inventory/docker/architecture_guard_fixture.go'
$guardPath = Join-Path $PSScriptRoot 'check-architecture.ps1'

if (Test-Path -LiteralPath $fixturePath) {
    throw "Refusing to overwrite existing architecture fixture: $fixturePath"
}

$modulePath = (& go list -m).Trim()
if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($modulePath)) {
    throw 'Unable to resolve the Go module path.'
}

try {
    @"
package docker

import _ "$modulePath/internal/inventory"
"@ | Set-Content -LiteralPath $fixturePath -Encoding utf8

    $output = & pwsh -NoProfile -File $guardPath 2>&1 | Out-String
    $guardExit = $LASTEXITCODE
    if ($guardExit -eq 0 -or $output -notmatch 'Inventory adapters must use application/domain contracts') {
        throw "The architecture guard did not reject the compatibility-facade fixture. Output:`n$output"
    }
    Write-Output 'Negative fixture rejected: Inventory adapters cannot import the root compatibility facade.'
}
finally {
    if (Test-Path -LiteralPath $fixturePath) {
        Remove-Item -LiteralPath $fixturePath
    }
}

& pwsh -NoProfile -File $guardPath
if ($LASTEXITCODE -ne 0) {
    throw 'The architecture guard failed after the negative fixture was removed.'
}
Write-Output 'Inventory architecture guard positive fixture passed.'
