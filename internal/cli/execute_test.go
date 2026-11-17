package cli

import (
	"bytes"
	"context"
	"io"
	"reflect"
	"strings"
	"testing"
)

type contextKey struct{}

func TestExecuteRoutesMonitoringAliasThroughExistingUseCase(t *testing.T) {
	type marker struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, marker{})
	wantArgs := []string{"integration", "list", "--format=json"}
	var gotArgs []string
	var gotContext context.Context

	code := Execute(ctx, []string{"monitoring", "instance", "list", "--format=json"}, io.Discard, io.Discard,
		func(commandContext context.Context, args []string, _, _ io.Writer) int {
			gotContext = commandContext
			gotArgs = append([]string(nil), args...)
			return 7
		}, func(io.Writer) {})

	if code != 7 {
		t.Fatalf("exit code = %d, want forwarded code 7", code)
	}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("dispatched args = %#v, want %#v", gotArgs, wantArgs)
	}
	if gotContext == nil || gotContext.Value(contextKey{}) == nil {
		t.Fatal("command context was not preserved through Cobra execution")
	}
}

func TestExecuteMonitoringAliasesMatchLegacyDispatch(t *testing.T) {
	tests := []struct {
		name       string
		legacyArgs []string
		aliasArgs  []string
	}{
		{name: "instance create alias", legacyArgs: []string{"integration", "configure", "--id", "cache"}, aliasArgs: []string{"monitoring", "instance", "create", "--id", "cache"}},
		{name: "instance configure", legacyArgs: []string{"integration", "configure", "--id", "cache"}, aliasArgs: []string{"monitoring", "instance", "configure", "--id", "cache"}},
		{name: "instance update", legacyArgs: []string{"integration", "update", "--id", "cache"}, aliasArgs: []string{"monitoring", "instance", "update", "--id", "cache"}},
		{name: "instance enable", legacyArgs: []string{"integration", "enable", "--id", "cache"}, aliasArgs: []string{"monitoring", "instance", "enable", "--id", "cache"}},
		{name: "instance disable", legacyArgs: []string{"integration", "disable", "--id", "cache"}, aliasArgs: []string{"monitoring", "instance", "disable", "--id", "cache"}},
		{name: "instance delete", legacyArgs: []string{"integration", "delete", "--id", "cache"}, aliasArgs: []string{"monitoring", "instance", "delete", "--id", "cache"}},
		{name: "instance list", legacyArgs: []string{"integration", "list", "--output", "json"}, aliasArgs: []string{"monitoring", "instance", "list", "--output", "json"}},
		{name: "instance inspect", legacyArgs: []string{"integration", "inspect", "--id", "cache"}, aliasArgs: []string{"monitoring", "instance", "inspect", "--id", "cache"}},
		{name: "instance status", legacyArgs: []string{"integration", "status", "--state-path", "agent.db"}, aliasArgs: []string{"monitoring", "instance", "status", "--state-path", "agent.db"}},
		{name: "instance history", legacyArgs: []string{"integration", "history", "--id", "cache"}, aliasArgs: []string{"monitoring", "instance", "history", "--id", "cache"}},
		{name: "instance retry", legacyArgs: []string{"integration", "retry", "--id", "cache"}, aliasArgs: []string{"monitoring", "instance", "retry", "--id", "cache"}},
		{name: "instance match", legacyArgs: []string{"integration", "match", "--id", "cache"}, aliasArgs: []string{"monitoring", "instance", "match", "--id", "cache"}},
		{name: "profile list", legacyArgs: []string{"integration", "catalog", "list"}, aliasArgs: []string{"monitoring", "profile", "list"}},
		{name: "profile inspect", legacyArgs: []string{"integration", "catalog", "inspect", "--id", "redis"}, aliasArgs: []string{"monitoring", "profile", "inspect", "--id", "redis"}},
		{name: "template create", legacyArgs: []string{"integration", "template", "create", "--file", "template.json"}, aliasArgs: []string{"monitoring", "template", "create", "--file", "template.json"}},
		{name: "template update", legacyArgs: []string{"integration", "template", "update", "--id", "local"}, aliasArgs: []string{"monitoring", "template", "update", "--id", "local"}},
		{name: "template list", legacyArgs: []string{"integration", "template", "list"}, aliasArgs: []string{"monitoring", "template", "list"}},
		{name: "template inspect", legacyArgs: []string{"integration", "template", "inspect", "--id", "local", "--output", "summary"}, aliasArgs: []string{"monitoring", "template", "inspect", "--id", "local", "--output", "summary"}},
		{name: "template enable", legacyArgs: []string{"integration", "template", "enable", "--id", "local"}, aliasArgs: []string{"monitoring", "template", "enable", "--id", "local"}},
		{name: "template disable", legacyArgs: []string{"integration", "template", "disable", "--id", "local"}, aliasArgs: []string{"monitoring", "template", "disable", "--id", "local"}},
		{name: "template delete", legacyArgs: []string{"integration", "template", "delete", "--id", "local"}, aliasArgs: []string{"monitoring", "template", "delete", "--id", "local"}},
		{name: "proposal list", legacyArgs: []string{"discovery", "proposals", "list"}, aliasArgs: []string{"monitoring", "proposal", "list"}},
		{name: "proposal inspect", legacyArgs: []string{"discovery", "proposals", "inspect", "--id", "proposal-1"}, aliasArgs: []string{"monitoring", "proposal", "inspect", "--id", "proposal-1"}},
		{name: "proposal dismiss", legacyArgs: []string{"discovery", "proposals", "dismiss", "--id", "proposal-1", "--revision", "3"}, aliasArgs: []string{"monitoring", "proposal", "dismiss", "--id", "proposal-1", "--revision", "3"}},
		{name: "proposal approve", legacyArgs: []string{"discovery", "proposals", "approve", "--id", "proposal-1", "--revision", "2"}, aliasArgs: []string{"monitoring", "proposal", "approve", "--id", "proposal-1", "--revision", "2"}},
	}

	dispatchArgs := func(args []string) []string {
		t.Helper()
		var got []string
		code := Execute(context.Background(), args, io.Discard, io.Discard,
			func(_ context.Context, dispatched []string, _, _ io.Writer) int {
				got = append([]string(nil), dispatched...)
				return 0
			}, func(io.Writer) {})
		if code != 0 {
			t.Fatalf("Execute(%v) exit code = %d, want 0", args, code)
		}
		return got
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			legacy := dispatchArgs(test.legacyArgs)
			alias := dispatchArgs(test.aliasArgs)
			if !reflect.DeepEqual(alias, legacy) {
				t.Fatalf("alias dispatch = %#v, legacy dispatch = %#v", alias, legacy)
			}
		})
	}
}

func TestExecuteListsExistingDependencyAndInventoryOperations(t *testing.T) {
	tests := []struct {
		name       string
		command    string
		operations []string
	}{
		{
			name:       "dependency",
			command:    "dependency",
			operations: []string{"list", "install", "status", "pending", "configure", "disable", "uninstall"},
		},
		{
			name:       "inventory",
			command:    "inventory",
			operations: []string{"list", "inspect", "status"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Execute(context.Background(), []string{test.command, "--help"}, &stdout, &stderr,
				func(context.Context, []string, io.Writer, io.Writer) int {
					t.Fatal("help must not dispatch a business command")
					return 0
				}, func(io.Writer) {})
			if code != 0 {
				t.Fatalf("help exit code = %d, want 0; stderr = %q", code, stderr.String())
			}
			for _, operation := range test.operations {
				if !strings.Contains(stdout.String(), operation) {
					t.Errorf("help output does not list %q: %s", operation, stdout.String())
				}
			}
		})
	}
}

func TestExecuteDependencyAndInventoryOperationsPreserveLegacyDispatch(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "dependency install flags", args: []string{"dependency", "install", "--state-path", "agent.db", "node-exporter"}},
		{name: "dependency configure flags", args: []string{"dependency", "configure", "--state-path", "agent.db"}},
		{name: "inventory inspect flags", args: []string{"inventory", "inspect", "--id", "container-1"}},
		{name: "legacy extension command", args: []string{"dependency", "future-command", "--raw", "value"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got []string
			code := Execute(context.Background(), test.args, io.Discard, io.Discard,
				func(_ context.Context, args []string, _, _ io.Writer) int {
					got = append([]string(nil), args...)
					return 7
				}, func(io.Writer) {})
			if code != 7 {
				t.Fatalf("exit code = %d, want legacy handler code 7", code)
			}
			if !reflect.DeepEqual(got, test.args) {
				t.Fatalf("dispatched args = %#v, want %#v", got, test.args)
			}
		})
	}
}

func TestExecuteDependencyLeafHelpDoesNotDispatch(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"dependency", "install", "--help"}, &stdout, &stderr,
		func(context.Context, []string, io.Writer, io.Writer) int {
			t.Fatal("leaf help must not dispatch a business command")
			return 0
		}, func(io.Writer) {})
	if code != 0 {
		t.Fatalf("help exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Usage:") {
		t.Fatalf("leaf help output lacks usage: %s", stdout.String())
	}
}
