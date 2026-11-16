package cli

import (
	"context"
	"io"
	"reflect"
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
		{name: "instance configure", legacyArgs: []string{"integration", "configure", "--id", "cache"}, aliasArgs: []string{"monitoring", "instance", "configure", "--id", "cache"}},
		{name: "instance status", legacyArgs: []string{"integration", "status", "--state-path", "agent.db"}, aliasArgs: []string{"monitoring", "instance", "status", "--state-path", "agent.db"}},
		{name: "profile inspect", legacyArgs: []string{"integration", "catalog", "inspect", "--id", "redis"}, aliasArgs: []string{"monitoring", "profile", "inspect", "--id", "redis"}},
		{name: "template update", legacyArgs: []string{"integration", "template", "update", "--id", "local"}, aliasArgs: []string{"monitoring", "template", "update", "--id", "local"}},
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
