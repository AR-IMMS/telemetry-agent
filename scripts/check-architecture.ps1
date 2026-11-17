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
        },
        @{
            SourceRoot = "$modulePath/internal/runtime/collector"
            TargetRoot = "$modulePath/internal/integration"
            Reason = 'Runtime compiles validated Monitoring contributions through its collector contracts; it does not depend on the compatibility facade.'
        }
    )

    # Reconciliation owns the consumer-side runtime ports. It may depend on
    # runtime contract values, but not on the concrete lifecycle package.
    $reconciliationContextRoot = "$modulePath/internal/configuration/reconciliation"
    $agentLifecyclePackage = "$modulePath/internal/runtime/collector/activation"
    $runtimeContextRoot = "$modulePath/internal/runtime"
    $healthContextRoot = "$modulePath/internal/health"
    $platformPackageRoot = "$modulePath/internal/platform"

    $enrollmentFacadeImportPath = "$modulePath/internal/identity/enrollment"
    $enrollmentDomain = "$enrollmentFacadeImportPath/domain"
    $enrollmentApplication = "$enrollmentFacadeImportPath/application"
    $installationFacadeImportPath = "$modulePath/internal/identity/installation"
    $installationDomain = "$installationFacadeImportPath/domain"
    $installationApplication = "$installationFacadeImportPath/application"
    $violations = [System.Collections.Generic.List[string]]::new()
    foreach ($layerPath in @(
        'internal/identity/enrollment/domain', 'internal/identity/enrollment/application',
        'internal/identity/installation/domain', 'internal/identity/installation/application'
    )) {
        if (-not (Test-Path -LiteralPath (Join-Path $repoRoot $layerPath) -PathType Container)) {
            $violations.Add("Identity enrollment/installation requires real domain and application layers: $layerPath")
        }
    }
    $runtimeServiceDomain = "$modulePath/internal/runtime/service/domain"
    if (-not (Test-Path -LiteralPath (Join-Path $repoRoot 'internal/runtime/service/domain') -PathType Container)) {
        $violations.Add('Runtime service ownership models require an explicit internal/runtime/service/domain package')
    }
    $agentStateModelsPath = Join-Path $repoRoot 'internal/agentstate/state.go'
    if (Test-Path -LiteralPath $agentStateModelsPath -PathType Leaf) {
        $agentStateModels = Get-Content -LiteralPath $agentStateModelsPath -Raw
        if ($agentStateModels -match '(?m)^type\s+(AgentInstallation|OwnedResource|OwnershipRecord)\s+struct\b') {
            $violations.Add('internal/agentstate/state.go must persist Runtime service ownership models from their owning package')
        }
    }
    $serviceDomainPackages = & go list -f '{{.ImportPath}}|{{range .Imports}}{{.}},{{end}}' ./internal/runtime/service/domain 2>$null
    if ($LASTEXITCODE -eq 0) {
        foreach ($line in $serviceDomainPackages) {
            $parts = $line -split '\|', 2
            if ($parts.Count -eq 2 -and $parts[1] -match [regex]::Escape("$modulePath/internal/agentstate")) {
                $violations.Add('Runtime service domain must not import the agentstate persistence adapter')
            }
        }
    }
    $runtimeServiceLayoutPath = Join-Path $repoRoot 'internal/runtime/service/layout.go'
    if (Test-Path -LiteralPath $runtimeServiceLayoutPath -PathType Leaf) {
        $source = Get-Content -LiteralPath $runtimeServiceLayoutPath -Raw
        if ($source -match 'agentstate\.(AgentInstallation|OwnedResource|OwnershipRecord)' -or
            $source -notmatch 'internal/runtime/service/domain') {
            $violations.Add('internal/runtime/service/layout.go must construct Runtime service domain ownership models')
        }
    }

    $packageLines = & go list -f '{{.ImportPath}}|{{range .Imports}}{{.}},{{end}}' ./internal/... ./cmd/agentctl
    if ($LASTEXITCODE -ne 0) {
        throw 'go list failed while checking architecture imports.'
    }

    # Runtime dependency catalog selection consumes only the enabled-name
    # snapshot it needs; it must not depend on the persisted multi-context state.
    $dependencyCatalog = Join-Path $repoRoot 'internal/runtime/dependencies/catalog.go'
    if (Test-Path -LiteralPath $dependencyCatalog -PathType Leaf) {
        $dependencyCatalogSource = Get-Content -LiteralPath $dependencyCatalog -Raw
        if ($dependencyCatalogSource -match '"github.com/ar-imms/telemetry-agent/internal/agentstate"') {
            $violations.Add('internal/runtime/dependencies/catalog.go must consume the enabled dependency-name contract, not internal/agentstate')
        }
    }
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
    # Integration CRUD is delivered through the desired-state use case; keep
    # the CLI adapter off Configuration's transitional root facade.
    $integrationCommandPath = Join-Path $repoRoot 'cmd/agentctl/integration_command.go'
    if (Test-Path -LiteralPath $integrationCommandPath -PathType Leaf) {
        $source = Get-Content -LiteralPath $integrationCommandPath -Raw
        if ($source -match '"github.com/ar-imms/telemetry-agent/internal/configuration"') {
            $violations.Add('cmd/agentctl/integration_command.go must use Configuration desiredstate/application instead of the root facade')
        }
        if ($source -match '"github.com/ar-imms/telemetry-agent/internal/integration"') {
            $violations.Add('cmd/agentctl/integration_command.go must resolve profiles through Monitoring definition/application instead of internal/integration')
        }
        if ($source -notmatch '"github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/application"') {
            $violations.Add('cmd/agentctl/integration_command.go must call the Configuration desiredstate application use case')
        }
    }
    # Template CLI operations call Configuration's owning use case directly.
    $templateCommandPath = Join-Path $repoRoot 'cmd/agentctl/integration_template_command.go'
    if (Test-Path -LiteralPath $templateCommandPath -PathType Leaf) {
        $source = Get-Content -LiteralPath $templateCommandPath -Raw
        if ($source -match '"github.com/ar-imms/telemetry-agent/internal/configuration"' -or
            $source -match '"github.com/ar-imms/telemetry-agent/internal/integration"') {
            $violations.Add('cmd/agentctl/integration_template_command.go must use Configuration templates/domain and application instead of compatibility facades')
        }
        foreach ($requiredImport in @(
            'internal/configuration/templates/application',
            'internal/configuration/templates/domain'
        )) {
            if ($source -notmatch [regex]::Escape($requiredImport)) {
                $violations.Add("cmd/agentctl/integration_template_command.go must use $requiredImport")
            }
        }
    }
    # Reconciliation consumes Configuration's snapshot and a narrow profile
    # contribution port; it must not depend on legacy Integration models.
    $reconciliationControllerPath = Join-Path $repoRoot 'internal/configuration/reconciliation/controller.go'
    if (Test-Path -LiteralPath $reconciliationControllerPath -PathType Leaf) {
        $source = Get-Content -LiteralPath $reconciliationControllerPath -Raw
        if ($source -match '"github.com/ar-imms/telemetry-agent/internal/integration"') {
            $violations.Add('Configuration reconciliation must use desiredstate and Monitoring contracts instead of internal/integration')
        }
        if ($source -notmatch '"github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"') {
            $violations.Add('Configuration reconciliation must consume the desiredstate domain model')
        }
    }
    # Proposal approval belongs to Configuration and consumes its canonical
    # desired-state model directly; the CLI is a delivery adapter over the same
    # model. Keep these paths off the temporary Integration compatibility facade.
    foreach ($file in @(
        'internal/configuration/proposals/approval.go',
        'internal/configuration/sqlite/approval.go',
        'cmd/agentctl/discovery_proposals_command.go'
    )) {
        $sourcePath = Join-Path $repoRoot $file
        if (Test-Path -LiteralPath $sourcePath -PathType Leaf) {
            $source = Get-Content -LiteralPath $sourcePath -Raw
            if ($source -match '"github.com/ar-imms/telemetry-agent/internal/integration"') {
                $violations.Add("$file must use Configuration desired-state contracts instead of internal/integration")
            }
        }
    }
    # Configuration adapters and CLI handlers consume proposal capabilities
    # directly, rather than depending on the retained legacy import facade.
    foreach ($file in @(
        'internal/configuration/sqlite/proposal.go',
        'internal/configuration/sqlite/approval.go',
        'internal/agentstate/sqlite_discovery.go',
        'internal/agentstate/sqlite_configuration.go',
        'cmd/agentctl/discovery_proposals_command.go',
        'cmd/agentctl/proposal_approval_runtime.go'
    )) {
        $sourcePath = Join-Path $repoRoot $file
        if (Test-Path -LiteralPath $sourcePath -PathType Leaf) {
            $source = Get-Content -LiteralPath $sourcePath -Raw
            if ($source -match '"github.com/ar-imms/telemetry-agent/internal/configuration/proposals"') {
                $violations.Add("$file must depend on Configuration proposal domain/application packages instead of the legacy facade")
            }
        }
    }
    $proposalApprovalRuntime = Join-Path $repoRoot 'cmd/agentctl/proposal_approval_runtime.go'
    if (Test-Path -LiteralPath $proposalApprovalRuntime -PathType Leaf) {
        $source = Get-Content -LiteralPath $proposalApprovalRuntime -Raw
        if ($source -match '"github.com/ar-imms/telemetry-agent/internal/integration"') {
            $violations.Add('cmd/agentctl/proposal_approval_runtime.go must use Monitoring matching and template application contracts instead of internal/integration')
        }
        if ($source -match '"github.com/ar-imms/telemetry-agent/internal/configuration"') {
            $violations.Add('cmd/agentctl/proposal_approval_runtime.go must use Configuration templates/application instead of the root facade')
        }
        foreach ($requiredImport in @(
            'internal/configuration/templates/application',
            'internal/monitoring/matching/application',
            'internal/monitoring/profiles/definitions/application',
            'internal/monitoring/templates/application'
        )) {
            if ($source -notmatch [regex]::Escape($requiredImport)) {
                $violations.Add("cmd/agentctl/proposal_approval_runtime.go must use $requiredImport")
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
    # Monitoring Template matching has real domain and application layers.
    # Narrow input/evidence contracts stay independent of Configuration's
    # persisted aggregate and all persistence / inventory adapters.
    $monitoringTemplatesRoot = "$modulePath/internal/monitoring/templates"
    $monitoringTemplatesDomain = "$monitoringTemplatesRoot/domain"
    $monitoringTemplatesApplication = "$monitoringTemplatesRoot/application"
    foreach ($layer in @('domain', 'application')) {
        $layerPath = Join-Path $repoRoot ("internal/monitoring/templates/{0}" -f $layer)
        if (-not (Test-Path -LiteralPath $layerPath -PathType Container)) {
            $violations.Add("Monitoring Templates requires a real $layer layer under internal/monitoring/templates")
        }
    }
    # Configuration Templates are a cohesive capability: the domain owns the
    # persisted template model; the application owns validation and CAS use cases.
    $configurationTemplatesRoot = "$modulePath/internal/configuration/templates"
    foreach ($layer in @('domain', 'application')) {
        $layerPath = Join-Path $repoRoot ("internal/configuration/templates/{0}" -f $layer)
        if (-not (Test-Path -LiteralPath $layerPath -PathType Container)) {
            $violations.Add("Configuration Templates requires a real $layer layer under internal/configuration/templates")
        }
    }
    $configurationTemplateDomain = "$configurationTemplatesRoot/domain"
    $configurationTemplateApplication = "$configurationTemplatesRoot/application"
    $monitoringDefinitionRoot = "$modulePath/internal/monitoring/profiles/definitions"
    $monitoringDefinitionDomain = "$monitoringDefinitionRoot/domain"
    $monitoringDefinitionApplication = "$monitoringDefinitionRoot/application"
    foreach ($layer in @('domain', 'application')) {
        $layerPath = Join-Path $repoRoot ("internal/monitoring/profiles/definitions/{0}" -f $layer)
        if (-not (Test-Path -LiteralPath $layerPath -PathType Container)) {
            $violations.Add("Monitoring definitions requires a real $layer layer under internal/monitoring/profiles/definitions")
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
        if ($package -eq $enrollmentDomain) {
            foreach ($imported in $imports) {
                if ($imported.StartsWith("$enrollmentApplication", [StringComparison]::Ordinal) -or
                    $imported -eq $enrollmentFacadeImportPath -or
                    $imported.StartsWith("$enrollmentFacadeImportPath/sqlite", [StringComparison]::Ordinal) -or
                    $imported.StartsWith("$modulePath/internal/coreapi", [StringComparison]::Ordinal) -or
                    $imported.StartsWith("$modulePath/internal/identity/credentials", [StringComparison]::Ordinal)) {
                    $violations.Add("Identity enrollment domain imports an outer layer or adapter: $package -> $imported")
                }
            }
        }
        if ($package -eq $enrollmentApplication) {
            foreach ($imported in $imports) {
                if ($imported -eq $enrollmentFacadeImportPath -or
                    $imported.StartsWith("$enrollmentFacadeImportPath/sqlite", [StringComparison]::Ordinal) -or
                    $imported.StartsWith("$modulePath/internal/coreapi", [StringComparison]::Ordinal) -or
                    $imported.StartsWith("$modulePath/internal/identity/credentials", [StringComparison]::Ordinal)) {
                    $violations.Add("Identity enrollment application imports a compatibility facade or adapter: $package -> $imported")
                }
            }
        }
        if ($imports -contains $enrollmentFacadeImportPath) {
            $violations.Add("Package $package imports the retired Identity enrollment facade; use domain/application owner packages")
        }
        if ($package -eq $installationDomain) {
            foreach ($imported in $imports) {
                if ($imported.StartsWith("$modulePath/internal/", [StringComparison]::Ordinal)) {
                    $violations.Add("Identity installation domain imports an outer layer or adapter: $package -> $imported")
                }
            }
        }
        if ($package -eq $installationApplication) {
            foreach ($imported in $imports) {
                if ($imported -eq $installationFacadeImportPath -or
                    $imported.StartsWith("$modulePath/internal/identity/sqlite", [StringComparison]::Ordinal) -or
                    $imported.StartsWith("$modulePath/internal/coreapi", [StringComparison]::Ordinal) -or
                    $imported.StartsWith("$modulePath/internal/identity/credentials", [StringComparison]::Ordinal)) {
                    $violations.Add("Identity installation application imports an adapter or compatibility facade: $package -> $imported")
                }
            }
        }
        if ($imports -contains $installationFacadeImportPath) {
            $violations.Add("Package $package imports the retired Identity installation facade; use domain/application owner packages")
        }
        if ($package -eq $monitoringDefinitionDomain) {
            foreach ($imported in $imports) {
                if ($imported.StartsWith("$monitoringDefinitionApplication", [StringComparison]::Ordinal) -or
                    $imported.StartsWith("$modulePath/internal/configuration", [StringComparison]::Ordinal) -or
                    $imported.StartsWith("$modulePath/internal/integration", [StringComparison]::Ordinal) -or
                    $imported.StartsWith("$modulePath/internal/inventory", [StringComparison]::Ordinal) -or
                    $imported.StartsWith("$modulePath/internal/runtime", [StringComparison]::Ordinal)) {
                    $violations.Add("Monitoring definition domain must not depend on application, Configuration, compatibility, Inventory, or Runtime packages: $package -> $imported")
                }
            }
        }
        if ($package -eq $monitoringDefinitionApplication) {
            foreach ($imported in $imports) {
                if ($imported.StartsWith("$modulePath/internal/configuration/sqlite", [StringComparison]::Ordinal) -or
                    $imported.StartsWith("$modulePath/internal/integration", [StringComparison]::Ordinal) -or
                    $imported.StartsWith("$modulePath/internal/inventory", [StringComparison]::Ordinal) -or
                    $imported.StartsWith("$modulePath/internal/runtime/collector/activation", [StringComparison]::Ordinal)) {
                    $violations.Add("Monitoring definition application must use domain/evidence contracts, not compatibility, persistence, Inventory, or activation adapters: $package -> $imported")
                }
            }
        }        if ($package -eq $monitoringDomain) {
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
        if ($package -eq $configurationTemplateDomain) {
            foreach ($imported in $imports) {
                if ($imported.StartsWith("$configurationTemplatesRoot/application", [StringComparison]::Ordinal) -or
                    $imported.StartsWith("$modulePath/internal/configuration/sqlite", [StringComparison]::Ordinal) -or
                    $imported.StartsWith("$modulePath/internal/integration", [StringComparison]::Ordinal)) {
                    $violations.Add("Configuration Templates domain imports an outer layer or compatibility facade: $package -> $imported")
                }
            }
        }
        if ($package -eq $configurationTemplateApplication) {
            foreach ($imported in $imports) {
                if ($imported.StartsWith("$modulePath/internal/configuration/sqlite", [StringComparison]::Ordinal) -or
                    $imported.StartsWith("$modulePath/internal/integration", [StringComparison]::Ordinal) -or
                    $imported.StartsWith("$modulePath/internal/inventory", [StringComparison]::Ordinal)) {
                    $violations.Add("Configuration Templates application must use owner contracts, not adapters, Inventory, or the Integration facade: $package -> $imported")
                }
            }
        }
        if ($package -eq $monitoringTemplatesApplication) {
            foreach ($imported in $imports) {
                if ($imported.StartsWith("$modulePath/internal/integration", [StringComparison]::Ordinal) -or
                    $imported.StartsWith("$modulePath/internal/configuration/sqlite", [StringComparison]::Ordinal) -or
                    $imported.StartsWith("$modulePath/internal/configuration", [StringComparison]::Ordinal) -or
                    $imported.StartsWith("$modulePath/internal/inventory", [StringComparison]::Ordinal)) {
                    $violations.Add("Monitoring Templates application must depend on its narrow evaluation contract, not Configuration, Integration, persistence, or Inventory packages: $package -> $imported")
                }
            }
        }
        if ($package -eq $monitoringTemplatesDomain) {
            foreach ($imported in $imports) {
                if ($imported.StartsWith("$modulePath/internal/configuration", [StringComparison]::Ordinal) -or
                    $imported.StartsWith("$modulePath/internal/integration", [StringComparison]::Ordinal) -or
                    $imported.StartsWith("$monitoringTemplatesApplication", [StringComparison]::Ordinal) -or
                    $imported.StartsWith("$monitoringTemplatesRoot/yaml", [StringComparison]::Ordinal)) {
                    $violations.Add("Monitoring Templates domain must not depend on Configuration or outer layers: $package -> $imported")
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
