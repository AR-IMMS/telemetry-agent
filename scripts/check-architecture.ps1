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
        "$modulePath/internal/identity",
        "$modulePath/internal/integration",
        "$modulePath/internal/monitoring/profiles",
        "$modulePath/internal/inventory",
        "$modulePath/internal/health",
        "$modulePath/internal/operations/command",
        "$modulePath/internal/operations/handlers"
    )
    $adapterRoots = @(
        "$modulePath/internal/runtime/service",
        "$modulePath/internal/runtime/collector/activation",
        "$modulePath/internal/agentstate",
        "$modulePath/internal/runtime/bootstrap",
        "$modulePath/internal/runtime/collector",
        "$modulePath/internal/coreapi",
        "$modulePath/internal/coreconfig",
        "$modulePath/internal/runtime/dependencies",
        "$modulePath/internal/runtime/opamp",
        "$modulePath/internal/runtime/process",
        "$modulePath/internal/configuration/sqlite",
        "$modulePath/internal/identity/credentials",
        "$modulePath/internal/identity/enrollment/sqlite",
        "$modulePath/internal/identity/sqlite",
        "$modulePath/internal/inventory/docker",
        "$modulePath/internal/inventory/sqlite",
        "$modulePath/internal/monitoring/profiles/yaml",
        "$modulePath/internal/runtime/collector/supervisor",
        "$modulePath/internal/operations/dockeradapter",
        "$modulePath/internal/operations/nats",
        "$modulePath/internal/operations/sqlite",
        "$modulePath/internal/operations/testsupport"
    )

    # Configuration consumes Runtime-owned immutable candidate/application
    # contracts during reconciliation. Concrete Collector Runtime packages remain
    # forbidden to Configuration.
    $contextContractImportAllowlist = @{
        "$modulePath/internal/configuration/reconciliation|$modulePath/internal/runtime/collector/contracts" = 'OD-05: Configuration owns reconciliation decisions and depends on Runtime only through the candidate/application contract.'
        "$modulePath/internal/health|$modulePath/internal/runtime/collector/contracts" = 'Runtime owns Collector lifecycle evidence; Health consumes the evidence contract to assess it.'
    }
    # One observed transition: identity currently reads the Windows registry
    # directly. Keep the exception specific and reasoned until an adapter exists.
    $transitionalImportAllowlist = @{
        "$modulePath/internal/identity/host|golang.org/x/sys/windows/registry" = 'Host identity package still contains the platform-specific registry adapter.'
    }
    $platformAdapterRoots = @('golang.org/x/sys/windows/registry')

    # Inventory reports provider evidence and must not depend on Monitoring profile policy.
    # The legacy Discovery package has been retired; do not retain a dead-path guard.
    $forbiddenContextEdges = @(
        @{
            SourceRoot = "$modulePath/internal/inventory"
            TargetRoot = "$modulePath/internal/monitoring/profiles"
            Reason = 'Inventory emits normalized observations; Monitoring owns profile interpretation.'
        },
        @{
            SourceRoot = "$modulePath/internal/inventory"
            TargetRoot = "$modulePath/internal/integration"
            Reason = 'Inventory supplies observations; Monitoring owns profile matching and recommendations.'
        }
    )

    # Reconciliation owns the consumer-side runtime ports. It may depend on
    # runtime contract values, but not on the concrete lifecycle package.
    $reconciliationContextRoot = "$modulePath/internal/configuration/reconciliation"
    $agentLifecyclePackage = "$modulePath/internal/runtime/collector/activation"
    $runtimeContextRoot = "$modulePath/internal/runtime"
    $healthContextRoot = "$modulePath/internal/health"
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
    # Profile matching and catalog CLI routes consume Monitoring directly after
    # their compatibility migration; keep them off the broad Integration facade.
    foreach ($file in @('cmd/agentctl/integration_matching.go', 'cmd/agentctl/integration_catalog_command.go')) {
        $sourcePath = Join-Path $repoRoot $file
        if (Test-Path -LiteralPath $sourcePath -PathType Leaf) {
            $source = Get-Content -LiteralPath $sourcePath -Raw
            if ($source -match '"github.com/ar-imms/telemetry-agent/internal/integration"') {
                $violations.Add("$file must use Monitoring-owned profile/matching packages instead of internal/integration")
            }
        }
    }
    # Ordered schema composition belongs to the Application boundary. The
    # Agent-state persistence context owns its base schema but does not select
    # or register other contexts' migrations.
    $migrationComposition = Join-Path $repoRoot 'internal/app/migrations/registry.go'
    if (-not (Test-Path -LiteralPath $migrationComposition -PathType Leaf)) {
        $violations.Add('Application-owned SQLite migration registry is missing at internal/app/migrations/registry.go')
    }
    $agentStateSchema = Join-Path $repoRoot 'internal/agentstate/sqlite_schema.go'
    if (Test-Path -LiteralPath $agentStateSchema -PathType Leaf) {
        $agentStateSchemaSource = Get-Content -LiteralPath $agentStateSchema -Raw
        if ($agentStateSchemaSource -match 'ensureSQLiteSchema|migrationForVersion|internal/(configuration|identity|inventory|operations|runtime)/.+/sqlite') {
            $violations.Add('Agentstate must not compose or import context-owned SQLite migrations; register them through internal/app/migrations')
        }
    }
    # Monitoring Profiles has completed its move under an explicit
    # Context -> Capability -> Layer structure. Keep the domain and application
    # packages real; the YAML directory is an input adapter.
    $monitoringProfilesRoot = "$modulePath/internal/monitoring/profiles"
    foreach ($layer in @('domain', 'application')) {
        $layerPath = Join-Path $repoRoot ("internal/monitoring/profiles/{0}" -f $layer)
        if (-not (Test-Path -LiteralPath $layerPath -PathType Container)) {
            $violations.Add("Monitoring Profiles requires a real $layer layer under internal/monitoring/profiles")
        }
    }
    $monitoringDomain = "$monitoringProfilesRoot/domain"
    $monitoringApplication = "$monitoringProfilesRoot/application"
    foreach ($line in $packageLines) {
        $parts = $line -split '\|', 2
        if ($parts.Count -ne 2) { continue }
        $package = $parts[0]
        $imports = @()
        if (-not [string]::IsNullOrWhiteSpace($parts[1])) {
            $imports = @($parts[1] -split ',' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
        }
        if ($package -eq $monitoringDomain) {
            foreach ($imported in $imports) {
                if ($imported.StartsWith("$modulePath/internal/inventory", [StringComparison]::Ordinal)) {
                    $violations.Add("Monitoring Profiles domain must translate Inventory evidence at the application boundary: $package -> $imported")
                }
                if ($imported.StartsWith("$monitoringProfilesRoot/application", [StringComparison]::Ordinal) -or
                    $imported.StartsWith("$monitoringProfilesRoot/yaml", [StringComparison]::Ordinal)) {
                    $violations.Add("Monitoring Profiles domain imports an outer layer: $package -> $imported")
                }
            }
        }
        if ($package -eq "$monitoringProfilesRoot/application") {
            foreach ($imported in $imports) {
                if ($imported.StartsWith("$modulePath/internal/inventory", [StringComparison]::Ordinal)) {
                    $violations.Add("Monitoring Profiles application must consume its endpoint evidence projection, not Inventory packages: $package -> $imported")
                }
            }
        }
        if ($package -eq $monitoringApplication) {
            foreach ($imported in $imports) {
                if ($imported.StartsWith("$monitoringProfilesRoot/yaml", [StringComparison]::Ordinal)) {
                    $violations.Add("Monitoring Profiles application imports an input adapter: $package -> $imported")
                }
            }
        }
        if ($package.StartsWith("$modulePath/internal/agentstate", [StringComparison]::Ordinal)) {
            foreach ($imported in $imports) {
                if ($imported.StartsWith("$modulePath/internal/configuration/remoteconfig/sqlite", [StringComparison]::Ordinal)) {
                    $violations.Add("Agent state must not import Configuration remote-config SQLite: $package -> $imported")
                }
            }
        }
    }
    $legacyMonitoringProfileRoot = Join-Path $repoRoot 'internal/monitoringprofile'
    if (Test-Path -LiteralPath $legacyMonitoringProfileRoot) {
        $legacyMonitoringProfileFiles = Get-ChildItem -LiteralPath $legacyMonitoringProfileRoot -Recurse -File -Filter '*.go'
        if ($legacyMonitoringProfileFiles.Count -gt 0) {
            $violations.Add('monitoring profile capability must live under internal/monitoring/profiles; legacy internal/monitoringprofile still contains Go source')
        }
    }
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

        foreach ($rule in $forbiddenContextEdges) {
            $isSourceContext = $package -eq $rule.SourceRoot -or
                $package.StartsWith("$($rule.SourceRoot)/", [StringComparison]::Ordinal)
            if (-not $isSourceContext) { continue }
            foreach ($imported in $imports) {
                if ($imported -eq $rule.TargetRoot -or
                    $imported.StartsWith("$($rule.TargetRoot)/", [StringComparison]::Ordinal)) {
                    $violations.Add("forbidden context dependency: $package -> $imported ($($rule.Reason))")
                }
            }
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

        $isRuntimePackage = $package -eq $runtimeContextRoot -or
            $package.StartsWith("$runtimeContextRoot/", [StringComparison]::Ordinal)
        if ($isRuntimePackage -and ($imports -contains $healthContextRoot -or
            @($imports | Where-Object { $_.StartsWith("$healthContextRoot/", [StringComparison]::Ordinal) }).Count -gt 0)) {
            $violations.Add("forbidden context dependency: $package -> $healthContextRoot (Runtime publishes Collector evidence through its contracts; Health consumes it.)")
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
                    if ($contextContractImportAllowlist.ContainsKey($edge) -and
                        $imported -eq "$modulePath/internal/runtime/collector/contracts") {
                        break
                    }
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
