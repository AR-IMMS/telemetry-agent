package opampserver

import (
	"context"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/coreapi"
	"github.com/ar-imms/telemetry-agent/internal/mockcore"
	"github.com/open-telemetry/opamp-go/protobufs"
)

func TestRemoteStatusWithoutObservedConfigHashIsNotReported(t *testing.T) {
	const token = "status-attribution-test-token"
	core, err := mockcore.New(mockcore.ProvisionedInstallation{
		AccessToken: token, InstallationID: "11111111-2222-4333-8444-555555555555",
		NodeID: "33333333-4444-4555-8666-777777777777", InstanceUID: "44444444-5555-4666-8777-888888888888",
		Policy: coreapi.DesiredGatewayPolicy{SchemaVersion: "ar-imms/gateway-policy/v1", Revision: 2, GatewayEndpoint: "127.0.0.1:4317"},
	})
	if err != nil {
		t.Fatalf("create Mock Core: %v", err)
	}
	coreHTTP := httptest.NewTLSServer(core)
	defer coreHTTP.Close()
	client, err := coreapi.NewAgentManagementClient(coreHTTP.URL, token, coreHTTP.Client())
	if err != nil {
		t.Fatalf("create Core client: %v", err)
	}
	adapter := &Server{options: Options{Logger: slog.Default()}, sent: make(map[string]map[string]int64)}
	adapter.reportRemoteStatus(context.Background(), client, "44444444-5555-4666-8777-888888888888", &protobufs.RemoteConfigStatus{
		Status:               protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED,
		LastRemoteConfigHash: []byte("01234567890123456789012345678901"),
	})
	if observations := core.Statuses(); len(observations) != 0 {
		t.Fatalf("unmapped remote config hash produced Core status: %+v", observations)
	}
}

func TestConfigRevisionHistoryIsBounded(t *testing.T) {
	adapter := &Server{sent: make(map[string]map[string]int64)}
	for revision := 1; revision <= 1000; revision++ {
		adapter.markSent("tracked-instance", fmt.Sprintf("%064x", revision), int64(revision))
	}
	if len(adapter.sent["tracked-instance"]) > 32 {
		t.Fatalf("tracked %d config hashes for one instance; want at most 32", len(adapter.sent["tracked-instance"]))
	}
	if revision := adapter.revisionForHash("tracked-instance", fmt.Sprintf("%064x", 1000)); revision != 1000 {
		t.Fatalf("latest config revision = %d, want 1000", revision)
	}
	for index := 1; index <= 1000; index++ {
		uid := fmt.Sprintf("%036x", index)
		hash := fmt.Sprintf("%064x", index)
		adapter.markSent(uid, hash, int64(index))
	}
	if len(adapter.sent) > 64 {
		t.Fatalf("tracked %d instances; want at most 64", len(adapter.sent))
	}
	if _, exists := adapter.sent[fmt.Sprintf("%036x", 1000)]; !exists {
		t.Fatal("latest active instance was evicted from config revision history")
	}
	for uid, revisions := range adapter.sent {
		if len(revisions) > 32 {
			t.Fatalf("tracked %d config hashes for %s; want at most 32", len(revisions), uid)
		}
	}
}
