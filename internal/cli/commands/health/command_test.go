package health

import (
	"context"
	"io"
	"reflect"
	"testing"

	"github.com/spf13/cobra"
)

func TestNewCommandsForwardExistingHealthPaths(t *testing.T) {
	for _, commandName := range []string{"status", "doctor"} {
		t.Run(commandName, func(t *testing.T) {
			var gotArgs []string
			var gotContext context.Context
			var gotStdout, gotStderr io.Writer
			gotExitCode := 0
			root := &cobra.Command{Use: "agentctl", SilenceErrors: true, SilenceUsage: true}
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.AddCommand(NewCommands(func(ctx context.Context, args []string, stdout, stderr io.Writer) int {
				gotContext = ctx
				gotArgs = append([]string(nil), args...)
				gotStdout, gotStderr = stdout, stderr
				return 9
			}, func(code int) { gotExitCode = code })...)

			ctx := context.WithValue(context.Background(), struct{}{}, "retained")
			root.SetArgs([]string{commandName, "--state-path", "agent.db"})
			if err := root.ExecuteContext(ctx); err != nil {
				t.Fatalf("ExecuteContext() error = %v", err)
			}

			if want := []string{commandName, "--state-path", "agent.db"}; !reflect.DeepEqual(gotArgs, want) {
				t.Fatalf("dispatch args = %#v, want %#v", gotArgs, want)
			}
			if gotContext != ctx {
				t.Fatal("dispatch did not receive the command context")
			}
			if gotStdout != root.OutOrStdout() || gotStderr != root.ErrOrStderr() {
				t.Fatal("dispatch did not receive configured output streams")
			}
			if gotExitCode != 9 {
				t.Fatalf("exit code = %d, want 9", gotExitCode)
			}
		})
	}
}
