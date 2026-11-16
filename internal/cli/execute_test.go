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
