package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"runtime"
	"sort"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/collectorruntime"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	"github.com/ar-imms/telemetry-agent/internal/integration"
)

type integrationCatalogView struct {
	Platform        string                      `json:"platform"`
	ArtifactVersion string                      `json:"artifact_version,omitempty"`
	ManifestVersion string                      `json:"manifest_version,omitempty"`
	Definitions     []integrationDefinitionView `json:"definitions"`
}

type integrationDefinitionView struct {
	ID                 string                                `json:"id"`
	Version            string                                `json:"version"`
	Matchable          bool                                  `json:"matchable"`
	Matching           *catalogMatchingView                  `json:"matching,omitempty"`
	TemplateSettings   map[string]catalogTemplateSettingView `json:"template_settings,omitempty"`
	Capabilities       capabilityView                        `json:"capabilities"`
	CapabilityMappings []catalogCapabilityMapping            `json:"capability_mappings,omitempty"`
}

type catalogTemplateSettingView struct {
	Type                     string   `json:"type"`
	Description              string   `json:"description"`
	Required                 bool     `json:"required"`
	Constraints              []string `json:"constraints,omitempty"`
	HasDefault               bool     `json:"has_default"`
	Default                  any      `json:"default"`
	DefaultOverridable       bool     `json:"default_overridable"`
	AllowResourceBinding     bool     `json:"allow_resource_binding"`
	BindingOverridesTemplate bool     `json:"binding_overrides_template"`
}

type catalogMatchingView struct {
	Mode                 string                     `json:"mode"`
	Selector             *integration.MatchSelector `json:"selector,omitempty"`
	RequiredCapabilities []string                   `json:"required_capabilities,omitempty"`
}

type catalogCapabilityMapping struct {
	ID         string   `json:"id"`
	NativeName string   `json:"native_name"`
	Kind       string   `json:"kind"`
	OS         []string `json:"os"`
	Signals    []string `json:"signals"`
	Pipeline   string   `json:"pipeline"`
}

func catalogTemplateSettingsView(rules map[string]integration.TemplateSettingRule) map[string]catalogTemplateSettingView {
	if len(rules) == 0 {
		return nil
	}
	settings := make(map[string]catalogTemplateSettingView, len(rules))
	for name, rule := range rules {
		settings[name] = catalogTemplateSettingView{
			Type: rule.Type, Description: rule.Description, Required: rule.Required,
			Constraints: append([]string(nil), rule.Constraints...), HasDefault: rule.Default != nil,
			Default: rule.Default, DefaultOverridable: rule.DefaultOverridable,
			AllowResourceBinding: rule.AllowResourceBinding, BindingOverridesTemplate: rule.BindingOverridesTemplate,
		}
	}
	return settings
}

func writeIntegrationCatalogHelp(w io.Writer) {
	fmt.Fprint(w, `Usage: agentctl integration catalog <list|inspect> [options]

list [--output text|json]
inspect --id <integration-id> --version <exact-version> [--output text|json]
The catalog is read-only. Capability availability is tied to the pinned artifact manifest for this platform.
`)
}

func runIntegrationCatalog(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" {
		writeIntegrationCatalogHelp(stdout)
		return 0
	}
	command := args[0]
	if command != "list" && command != "inspect" {
		return usageIntegrationCatalog(stderr, "unknown catalog command "+command)
	}
	flags := flag.NewFlagSet("integration catalog "+command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	id := flags.String("id", "", "exact Integration Definition ID")
	version := flags.String("version", "", "exact Integration Definition version")
	output := flags.String("output", "text", "output format: text or json")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if flags.NArg() != 0 || (*output != "text" && *output != "json") {
		return usageIntegrationCatalog(stderr, "unexpected argument or unsupported output format")
	}
	if command == "list" && (*id != "" || *version != "") {
		return usageIntegrationCatalog(stderr, "list does not accept --id or --version")
	}
	if command == "inspect" && (*id == "" || *version == "") {
		return usageIntegrationCatalog(stderr, "inspect requires --id and --version")
	}
	if err := ctx.Err(); err != nil {
		return integrationCommandError(stderr, "catalog "+command, err)
	}
	registry, err := productionIntegrationRegistry()
	if err != nil {
		return integrationCommandError(stderr, "catalog "+command, err)
	}
	definitions := registry.Definitions()
	if command == "inspect" {
		definition, err := registry.ResolveDefinition(*id, *version)
		if err != nil {
			return integrationCommandError(stderr, "catalog inspect", fmt.Errorf("%w; no matching Definition is shipped in this Agent build, use catalog list or the Integration Developer Guide", err))
		}
		definitions = []integration.IntegrationDefinition{definition}
	}
	view := buildIntegrationCatalogView(definitions)
	if *output == "json" {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(view); err != nil {
			return integrationCommandError(stderr, "catalog "+command, err)
		}
		return 0
	}
	writeIntegrationCatalogText(stdout, view)
	return 0
}

func buildIntegrationCatalogView(definitions []integration.IntegrationDefinition) integrationCatalogView {
	platform := identity.PlatformInfo{OS: runtime.GOOS, Architecture: runtime.GOARCH}
	view := integrationCatalogView{Platform: platform.OS + "/" + platform.Architecture, Definitions: make([]integrationDefinitionView, 0, len(definitions))}
	catalog, catalogErr := collectorruntime.StaticCatalog(platform)
	if catalogErr == nil {
		view.ArtifactVersion = catalog.ArtifactVersion()
		view.ManifestVersion = catalog.ManifestVersion()
	}
	for _, definition := range definitions {
		item := integrationDefinitionView{
			ID: definition.ID, Version: definition.Version,
			Matchable:        definition.Matching != nil,
			TemplateSettings: catalogTemplateSettingsView(definition.TemplateSettingRules),
			Capabilities:     capabilityView{Status: integration.CapabilityNotAssessed, Platform: view.Platform},
		}
		if definition.Matching != nil {
			mode := "custom"
			if definition.Matching.Selector != nil {
				mode = "selector"
			}
			item.Matching = &catalogMatchingView{
				Mode: mode, Selector: definition.Matching.Selector,
				RequiredCapabilities: append([]string(nil), definition.Matching.RequiredCapabilities...),
			}
			item.Capabilities.Required = append([]string(nil), definition.Matching.RequiredCapabilities...)
			if len(item.Capabilities.Required) > 0 {
				item.Capabilities.Status = integration.CapabilityUnverified
				if catalogErr == nil {
					assessment, err := (catalogCapabilityAssessor{catalog: catalog, platform: view.Platform}).Assess(item.Capabilities.Required)
					if err == nil {
						item.Capabilities = capabilityView{
							Status: assessment.Status, Platform: assessment.Platform,
							ArtifactVersion: assessment.ArtifactVersion, ManifestVersion: assessment.ManifestVersion,
							Required: assessment.Required, Unsupported: assessment.Unsupported,
						}
					}
				}
			}
			item.CapabilityMappings = capabilityMappingsFor(item.Capabilities.Required, catalog, catalogErr)
		}
		view.Definitions = append(view.Definitions, item)
	}
	return view
}

func capabilityMappingsFor(required []string, catalog collectorruntime.Catalog, catalogErr error) []catalogCapabilityMapping {
	if catalogErr != nil || len(required) == 0 {
		return nil
	}
	wanted := make(map[string]struct{}, len(required))
	for _, id := range required {
		wanted[id] = struct{}{}
	}
	var mappings []catalogCapabilityMapping
	for _, capability := range catalog.Manifest() {
		if _, ok := wanted[capability.ID]; ok {
			mappings = append(mappings, catalogCapabilityMapping{
				ID: capability.ID, NativeName: capability.NativeName, Kind: capability.Kind,
				OS: capability.OS, Signals: capability.Signals, Pipeline: capability.Pipeline,
			})
		}
	}
	return mappings
}

func writeIntegrationCatalogText(w io.Writer, view integrationCatalogView) {
	fmt.Fprintf(w, "Platform: %s", view.Platform)
	if view.ArtifactVersion != "" {
		fmt.Fprintf(w, " Collector: %s manifest: %s", view.ArtifactVersion, view.ManifestVersion)
	} else {
		fmt.Fprint(w, " Collector capability availability: unverified")
	}
	fmt.Fprintln(w)
	for _, definition := range view.Definitions {
		fmt.Fprintf(w, "%s@%s matchable=%t capability=%s", definition.ID, definition.Version, definition.Matchable, definition.Capabilities.Status)
		if len(definition.Capabilities.Required) > 0 {
			fmt.Fprintf(w, " required=%s", strings.Join(definition.Capabilities.Required, ","))
		}
		fmt.Fprintln(w)
		if definition.Matching != nil {
			fmt.Fprintf(w, "  matching mode=%s", definition.Matching.Mode)
			if definition.Matching.Selector != nil {
				selector, _ := json.Marshal(definition.Matching.Selector)
				fmt.Fprintf(w, " selector=%s", selector)
			}
			fmt.Fprintln(w)
		}
		for _, mapping := range definition.CapabilityMappings {
			fmt.Fprintf(w, "  capability %s -> %s %s pipeline=%s signals=%s platforms=%s\n", mapping.ID, mapping.Kind, mapping.NativeName, mapping.Pipeline, strings.Join(mapping.Signals, ","), strings.Join(mapping.OS, ","))
		}
		settingNames := make([]string, 0, len(definition.TemplateSettings))
		for name := range definition.TemplateSettings {
			settingNames = append(settingNames, name)
		}
		sort.Strings(settingNames)
		for _, name := range settingNames {
			rule := definition.TemplateSettings[name]
			fmt.Fprintf(w, "  setting %s type=%s required=%t description=%q", name, rule.Type, rule.Required, rule.Description)
			if rule.HasDefault {
				fmt.Fprintf(w, " default=%v overridable=%t", rule.Default, rule.DefaultOverridable)
			}
			if len(rule.Constraints) > 0 {
				fmt.Fprintf(w, " constraints=%q", strings.Join(rule.Constraints, "; "))
			}
			if rule.AllowResourceBinding {
				fmt.Fprint(w, " resource_binding=allowed")
			}
			fmt.Fprintln(w)
		}
	}
}

func usageIntegrationCatalog(w io.Writer, message string) int {
	fmt.Fprintln(w, "usage error: "+message)
	writeIntegrationCatalogHelp(w)
	return 2
}
