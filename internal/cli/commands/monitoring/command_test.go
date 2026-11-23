package monitoring

import (
	"context"
	"io"
	"reflect"
	"testing"
)

func TestNewCommandForwardsInstanceCreateToLegacyConfigure(t *testing.T) {
	var gotArgs []string
	var gotExitCode int
	command := NewCommand(
		func(_ context.Context, args []string, _, _ io.Writer) int {
			gotArgs = append([]string(nil), args...)
			return 7
		},
		func(code int) { gotExitCode = code },
	)
	command.SetArgs([]string{"instance", "create", "--id", "cache"})
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)

	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if !reflect.DeepEqual(gotArgs, []string{"integration", "configure", "--id", "cache"}) {
		t.Fatalf("dispatched args = %#v, want integration configure with unchanged arguments", gotArgs)
	}
	if gotExitCode != 7 {
		t.Fatalf("exit code = %d, want legacy handler result 7", gotExitCode)
	}
}
