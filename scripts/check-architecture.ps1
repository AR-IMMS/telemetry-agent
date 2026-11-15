$ErrorActionPreference = 'Stop'
$repoRoot = [System.IO.Path]::GetFullPath((Split-Path -Parent $PSScriptRoot))
$repoPrefix = $repoRoot.TrimEnd('\', '/') + [System.IO.Path]::DirectorySeparatorChar

function Get-RepoRelativePath([string]$Path) {
    $fullPath = [System.IO.Path]::GetFullPath($Path)
    if (-not $fullPath.StartsWith($repoPrefix, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "Path is outside the repository root: $fullPath"
    }
    return $fullPath.Substring($repoPrefix.Length).Replace('\', '/')
}

Push-Location $repoRoot
try {
    $modulePath = (& go list -m).Trim()
    if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($modulePath)) {
        throw 'Unable to resolve the Go module path.'
    }

    # These existing contexts own domain/application rules. They may consume
    # other context contracts, but must not import persistence, OS lifecycle,
    # Collector execution, or transport adapters directly.
    $domainRoots = @(
        "$modulePath/internal/configuration",
        "$modulePath/internal/discovery",
        "$modulePath/internal/identity",
        "$modulePath/internal/integration",
        "$modulePath/internal/inventory",
        "$modulePath/internal/monitoringprofile"
    )
    $adapterRoots = @(
        "$modulePath/internal/agentinstallation",
        "$modulePath/internal/agentlifecycle",
        "$modulePath/internal/agentstate",
        "$modulePath/internal/bootstrap",
        "$modulePath/internal/collectorruntime",
        "$modulePath/internal/coreapi",
        "$modulePath/internal/coreconfig",
        "$modulePath/internal/dependency",
        "$modulePath/internal/discovery/sqlite",
        "$modulePath/internal/configuration/sqlite",
        "$modulePath/internal/identity/credentials",
        "$modulePath/internal/identity/enrollment/sqlite",
        "$modulePath/internal/identity/sqlite",
        "$modulePath/internal/inventory/docker",
        "$modulePath/internal/inventory/sqlite",
        "$modulePath/internal/reconciliation",
        "$modulePath/internal/supervisor"
    )

    # One observed transition: identity currently reads the Windows registry
    # directly. Keep the exception specific and reasoned until an adapter exists.
    $transitionalImportAllowlist = @{
        "$modulePath/internal/identity/host|golang.org/x/sys/windows/registry" = 'Host identity package still contains the platform-specific registry adapter.'
    }
    $platformAdapterRoots = @('golang.org/x/sys/windows/registry')

    # Reconciliation owns the consumer-side runtime ports. It may depend on
    # runtime contract values, but not on the concrete lifecycle package.
    $reconciliationContextRoot = "$modulePath/internal/reconciliation"
    $agentLifecyclePackage = "$modulePath/internal/agentlifecycle"
    $platformPackageRoot = "$modulePath/internal/platform"

    # This pre-existing file is 454 lines. Avoid a mechanical split in the
    # foundation checkpoint; it may not grow and should be reduced in a
    # behavior-preserving context migration.
    $transitionalSizeAllowlist = @{
        'internal/identity/enrollment/service.go' = @{ MaxLines = 454; Reason = 'Pre-existing enrollment service; split only with a meaningful ownership boundary.' }
    }

    $packageLines = & go list -f '{{.ImportPath}}|{{range .Imports}}{{.}},{{end}}' ./internal/... ./cmd/agentctl
    if ($LASTEXITCODE -ne 0) {
        throw 'go list failed while checking architecture imports.'
    }

    $violations = [System.Collections.Generic.List[string]]::new()
    $packageDirectories = Get-ChildItem -Path (Join-Path $repoRoot 'cmd'), (Join-Path $repoRoot 'internal') -Recurse -File -Filter '*.go' |
        Group-Object DirectoryName
    foreach ($packageDirectory in $packageDirectories) {
        if (-not (Test-Path -LiteralPath (Join-Path $packageDirectory.Name 'doc.go'))) {
            $relativeDirectory = Get-RepoRelativePath $packageDirectory.Name
            $violations.Add("Go package lacks required doc.go: $relativeDirectory")
        }
    }
    foreach ($line in $packageLines) {
        $parts = $line -split '\|', 2
        if ($parts.Count -ne 2) { continue }
        $package = $parts[0]
        $imports = @()
        if (-not [string]::IsNullOrWhiteSpace($parts[1])) {
            $imports = @($parts[1] -split ',' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
        }

        $isPlatformPackage = $package -eq $platformPackageRoot -or
            $package.StartsWith("$platformPackageRoot/", [StringComparison]::Ordinal)
        if ($isPlatformPackage) {
            foreach ($imported in $imports) {
                if ($imported.StartsWith("$modulePath/internal/", [StringComparison]::Ordinal) -and
                    -not $imported.StartsWith("$platformPackageRoot/", [StringComparison]::Ordinal)) {
                    $violations.Add("platform package imports a business or application package: $package -> $imported")
                }
            }
        }

        $isReconciliationPackage = $package -eq $reconciliationContextRoot -or
            $package.StartsWith("$reconciliationContextRoot/", [StringComparison]::Ordinal)
        if ($isReconciliationPackage -and $imports -contains $agentLifecyclePackage) {
            $violations.Add("forbidden context dependency: $package -> $agentLifecyclePackage (Reconciliation owns its CandidateApplier and baseline ports; runtime implements them.)")
        }

        if ($package.StartsWith("$modulePath/internal/", [StringComparison]::Ordinal)) {
            $entrypoint = "$modulePath/cmd/agentctl"
            if ($imports -contains $entrypoint) {
                $violations.Add("$package imports the composition root $entrypoint")
            }
        }

        # Nested persistence/provider packages are adapters, not domain packages.
        # Their parent context remains guarded when it imports these adapters.
        $isContextAdapter = $false
        foreach ($adapter in $adapterRoots) {
            if ($package -eq $adapter -or $package.StartsWith("$adapter/", [StringComparison]::Ordinal)) {
                $isContextAdapter = $true
                break
            }
        }
        if ($isContextAdapter) { continue }
        $isDomain = $false
        foreach ($root in $domainRoots) {
            if ($package -eq $root -or $package.StartsWith("$root/", [StringComparison]::Ordinal)) {
                $isDomain = $true
                break
            }
        }
        if (-not $isDomain) { continue }

        foreach ($imported in $imports) {
            $edge = "$package|$imported"
            if ($transitionalImportAllowlist.ContainsKey($edge)) { continue }

            foreach ($platformAdapter in $platformAdapterRoots) {
                if ($imported -eq $platformAdapter -or $imported.StartsWith("$platformAdapter/", [StringComparison]::Ordinal)) {
                    $violations.Add("domain package imports platform adapter without an allowlist entry: $package -> $imported")
                    break
                }
            }
            foreach ($adapter in $adapterRoots) {
                if ($imported -eq $adapter -or $imported.StartsWith("$adapter/", [StringComparison]::Ordinal)) {
                    $violations.Add("domain package imports adapter: $package -> $imported")
                    break
                }
            }
        }
    }

    $goFiles = Get-ChildItem -Path (Join-Path $repoRoot 'cmd'), (Join-Path $repoRoot 'internal') -Recurse -File -Filter '*.go' |
        Where-Object { $_.Name -notlike '*_test.go' }
    $oversized = [System.Collections.Generic.List[string]]::new()
    foreach ($file in $goFiles) {
        $firstLines = Get-Content -LiteralPath $file.FullName -TotalCount 3
        if (($firstLines -join "`n") -match '// Code generated .* DO NOT EDIT\.') { continue }
        $lineCount = [System.IO.File]::ReadAllLines($file.FullName).Length
        if ($lineCount -gt 450) {
            $relativePath = Get-RepoRelativePath $file.FullName
            if ($transitionalSizeAllowlist.ContainsKey($relativePath)) {
                $allowance = $transitionalSizeAllowlist[$relativePath]
                if ($lineCount -gt $allowance.MaxLines) {
                    $oversized.Add("$relativePath grew to $lineCount lines (transitional maximum $($allowance.MaxLines)): $($allowance.Reason)")
                } else {
                    Write-Output "Transitional size allowance: $relativePath has $lineCount/$($allowance.MaxLines) lines; $($allowance.Reason)"
                }
            } else {
                $oversized.Add("$relativePath has $lineCount lines (maximum 450)")
            }
        }
    }

    if ($violations.Count -gt 0 -or $oversized.Count -gt 0) {
        @($violations) + @($oversized) | ForEach-Object { Write-Error $_ }
        exit 1
    }

    Write-Output "Package doc.go check passed for $($packageDirectories.Count) source directories."
    Write-Output "Architecture import boundaries passed for $($packageLines.Count) packages."
    Write-Output "Production Go file size passed for $($goFiles.Count) files (generated and test files excluded)."
} finally {
    Pop-Location
}
