package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/configuration"
	"github.com/ar-imms/telemetry-agent/internal/integration"
)

func writeIntegrationTemplateHelp(w io.Writer) {
	fmt.Fprint(w, `Usage: agentctl integration template <create|update|list|inspect|enable|disable|delete> [options]

create/update accept one secret-free Template JSON object with --file <path> or --template-json <object>.
inspect prints a summary by default; use --output json to export the full stored Template.
Mutations require --expected-revision; create uses revision 0. Delete retains a tombstone.
`)
}

func runIntegrationTemplate(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" {
		writeIntegrationTemplateHelp(stdout)
		return 0
	}
	command := args[0]
	flags := flag.NewFlagSet("integration template "+command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	statePath := flags.String("state-path", agentstate.DefaultPath(), "persistent Agent state path")
	id := flags.String("id", "", "stable Template ID")
	expected := flags.Uint64("expected-revision", 0, "expected Template revision; create uses 0")
	templateJSON := flags.String("template-json", "", "secret-free Integration Template JSON")
	templateFile := flags.String("file", "", "path to a secret-free Integration Template JSON file")
	outputFormat := flags.String("output", "summary", "inspect output format: summary or json")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if flags.NArg() != 0 || strings.TrimSpace(*statePath) == "" {
		return usageIntegrationTemplate(stderr, "unexpected argument or empty state path")
	}
	if command != "create" && command != "update" && *templateFile != "" {
		return usageIntegrationTemplate(stderr, "--file is supported only by create and update")
	}
	if command != "inspect" && *outputFormat != "summary" {
		return usageIntegrationTemplate(stderr, "--output is supported only by inspect")
	}
	if command == "inspect" && *outputFormat != "summary" && *outputFormat != "json" {
		return usageIntegrationTemplate(stderr, "inspect --output must be summary or json")
	}
	if err := ctx.Err(); err != nil {
		return integrationCommandError(stderr, "template "+command, err)
	}
	store := agentstate.NewSQLiteStore(*statePath)
	repository := store.TemplateRepository()
	registry, err := productionIntegrationRegistry()
	if err != nil {
		return integrationCommandError(stderr, "template "+command, err)
	}
	service := configuration.NewTemplateService(registry, repository)
	switch command {
	case "create", "update":
		if flagProvided(args[1:], "--file") == flagProvided(args[1:], "--template-json") {
			return usageIntegrationTemplate(stderr, "specify exactly one of --file or --template-json")
		}
		if command == "update" && !flagProvided(args[1:], "--expected-revision") {
			return usageIntegrationTemplate(stderr, "update requires --expected-revision")
		}
		if command == "create" && *expected != 0 {
			return usageIntegrationTemplate(stderr, "create requires --expected-revision 0")
		}
		var template integration.IntegrationTemplate
		if *templateFile != "" {
			template, err = decodeTemplateFile(*templateFile)
		} else {
			template, err = decodeTemplate(*templateJSON)
		}
		if err != nil {
			return integrationCommandError(stderr, "template "+command, err)
		}
		if template.Owner != "" && template.Owner != integration.TemplateOwnerLocal {
			return integrationCommandError(stderr, "template "+command, configuration.ErrTemplateOwnerNotLocal)
		}
		template.Owner = integration.TemplateOwnerLocal
		stored, setRevision, err := service.Put(template, *expected)
		if err != nil {
			return integrationCommandError(stderr, "template "+command, err)
		}
		fmt.Fprintf(stdout, "Integration Template committed: id=%s revision=%d status=%s template_set_revision=%d\n", stored.ID, stored.Revision, stored.Status, setRevision)
	case "list":
		snapshot, err := service.Snapshot()
		if err != nil {
			return integrationCommandError(stderr, "template list", err)
		}
		fmt.Fprintf(stdout, "Template set revision: %d\n", snapshot.Revision)
		for _, template := range snapshot.Templates {
			fmt.Fprintf(stdout, "%s %s@%s revision=%d status=%s\n", template.ID, template.DefinitionID, template.DefinitionVersion, template.Revision, template.Status)
		}
	case "inspect":
		if *id == "" {
			return usageIntegrationTemplate(stderr, "inspect requires --id")
		}
		snapshot, err := service.Snapshot()
		if err != nil {
			return integrationCommandError(stderr, "template inspect", err)
		}
		for _, template := range snapshot.Templates {
			if template.ID == *id {
				if *outputFormat == "json" {
					if err := integration.ValidateIntegrationTemplate(template); err != nil {
						return integrationCommandError(stderr, "template inspect", err)
					}
					encoder := json.NewEncoder(stdout)
					encoder.SetIndent("", "  ")
					if err := encoder.Encode(template); err != nil {
						return integrationCommandError(stderr, "template inspect", err)
					}
					return 0
				}
				fmt.Fprintf(stdout, "id=%s definition=%s@%s revision=%d status=%s settings=%d bindings=%d\n", template.ID, template.DefinitionID, template.DefinitionVersion, template.Revision, template.Status, len(template.Settings), len(template.Bindings))
				return 0
			}
		}
		return integrationCommandError(stderr, "template inspect", configuration.ErrTemplateNotFound)
	case "enable", "disable":
		if *id == "" || !flagProvided(args[1:], "--expected-revision") {
			return usageIntegrationTemplate(stderr, command+" requires --id and --expected-revision")
		}
		stored, setRevision, err := service.SetEnabled(*id, command == "enable", *expected)
		if err != nil {
			return integrationCommandError(stderr, "template "+command, err)
		}
		fmt.Fprintf(stdout, "Integration Template %s: id=%s revision=%d template_set_revision=%d\n", command+"d", stored.ID, stored.Revision, setRevision)
	case "delete":
		if *id == "" || !flagProvided(args[1:], "--expected-revision") {
			return usageIntegrationTemplate(stderr, "delete requires --id and --expected-revision")
		}
		stored, setRevision, err := service.Delete(*id, *expected)
		if err != nil {
			return integrationCommandError(stderr, "template delete", err)
		}
		fmt.Fprintf(stdout, "Integration Template tombstoned: id=%s revision=%d template_set_revision=%d\n", stored.ID, stored.Revision, setRevision)
	default:
		return usageIntegrationTemplate(stderr, "unknown Template command "+command)
	}
	return 0
}

func decodeTemplate(raw string) (integration.IntegrationTemplate, error) {
	if strings.TrimSpace(raw) == "" {
		return integration.IntegrationTemplate{}, fmt.Errorf("--template-json is required")
	}
	return decodeTemplateReader(strings.NewReader(raw))
}

func decodeTemplateFile(path string) (integration.IntegrationTemplate, error) {
	if strings.TrimSpace(path) == "" {
		return integration.IntegrationTemplate{}, fmt.Errorf("--file path is required")
	}
	file, err := os.Open(path)
	if err != nil {
		return integration.IntegrationTemplate{}, fmt.Errorf("open Template file: %w", err)
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil {
		return integration.IntegrationTemplate{}, fmt.Errorf("read Template file: %w", err)
	}
	if len(content) > 1<<20 {
		return integration.IntegrationTemplate{}, fmt.Errorf("Template JSON exceeds the 1 MiB file limit")
	}
	return decodeTemplateReader(bytes.NewReader(content))
}

func decodeTemplateReader(input io.Reader) (integration.IntegrationTemplate, error) {
	decoder := json.NewDecoder(input)
	decoder.DisallowUnknownFields()
	var template integration.IntegrationTemplate
	if err := decoder.Decode(&template); err != nil {
		return integration.IntegrationTemplate{}, fmt.Errorf("decode Template JSON (maximum 1 MiB): %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return integration.IntegrationTemplate{}, fmt.Errorf("Template JSON must contain one object")
	}
	return template, nil
}

func usageIntegrationTemplate(w io.Writer, message string) int {
	fmt.Fprintln(w, "usage error: "+message)
	writeIntegrationTemplateHelp(w)
	return 2
}
