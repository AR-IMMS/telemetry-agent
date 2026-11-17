package inventory

import (
	"bytes"
	"context"
	"io"
	"reflect"
	"testing"

	domain "github.com/ar-imms/telemetry-agent/internal/inventory/observation/domain"
	"github.com/spf13/cobra"
)

func TestNewCommandForwardsLegacyPathAndFlags(t *testing.T) {
	var gotArgs []string
	var gotContext context.Context
	var gotStdout, gotStderr io.Writer
	gotExitCode := 0
	root := &cobra.Command{Use: "agentctl", SilenceErrors: true, SilenceUsage: true}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.AddCommand(NewCommand(func(ctx context.Context, args []string, stdout, stderr io.Writer) int {
		gotContext = ctx
		gotArgs = append([]string(nil), args...)
		gotStdout, gotStderr = stdout, stderr
		return 7
	}, func(code int) { gotExitCode = code }))

	ctx := context.WithValue(context.Background(), struct{}{}, "retained")
	root.SetArgs([]string{"inventory", "inspect", "docker/local/container-1", "--state-path", "custom.db"})
	if err := root.ExecuteContext(ctx); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}

	if want := []string{"inventory", "inspect", "docker/local/container-1", "--state-path", "custom.db"}; !reflect.DeepEqual(gotArgs, want) {
		t.Fatalf("dispatch args = %#v, want %#v", gotArgs, want)
	}
	if gotContext != ctx {
		t.Fatal("dispatch did not receive the command context")
	}
	if gotStdout != root.OutOrStdout() || gotStderr != root.ErrOrStderr() {
		t.Fatal("dispatch did not receive the configured output streams")
	}
	if gotExitCode != 7 {
		t.Fatalf("exit code = %d, want 7", gotExitCode)
	}
}

func TestExecuteListUsesInjectedSnapshotAndPreservesColumns(t *testing.T) {
	var loadedPath string
	deps := Dependencies{
		DefaultStatePath: func() string { return "default.db" },
		LoadSnapshot: func(path string) (domain.InventorySnapshot, error) {
			loadedPath = path
			return domain.InventorySnapshot{Revision: 8, Resources: []domain.Resource{{
				Key:  domain.ResourceKey{Provider: "docker", ProviderInstance: "local", ProviderResourceID: "container-1"},
				Kind: domain.ResourceContainer, DisplayName: "redis", LifecycleState: "running", Presence: domain.PresencePresent,
			}}, Providers: []domain.ProviderStatus{}}, nil
		},
	}
	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"list"}, &stdout, &stderr, deps)
	if code != 0 {
		t.Fatalf("Execute() = %d, stderr=%q", code, stderr.String())
	}
	if loadedPath != "default.db" {
		t.Fatalf("snapshot path = %q, want default.db", loadedPath)
	}
	for _, want := range []string{"PROVIDER\tRESOURCE\tKIND\tNAME\tSTATE\tPRESENCE\tFRESHNESS", "docker\tdocker/local/container-1\tcontainer\tredis\trunning\tpresent"} {
		if !bytes.Contains(stdout.Bytes(), []byte(want)) {
			t.Errorf("output %q does not contain %q", stdout.String(), want)
		}
	}
}
