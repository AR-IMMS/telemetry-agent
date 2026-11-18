package mockfleet

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	opampclient "github.com/open-telemetry/opamp-go/client"
	clienttypes "github.com/open-telemetry/opamp-go/client/types"
	"github.com/open-telemetry/opamp-go/protobufs"
)

func TestRevokedEnrollmentCredentialDisconnectsAndRejectsOpAMPSession(t *testing.T) {
	fleet := New()
	if err := fleet.SeedInvitation("invitation-token", "node-1"); err != nil {
		t.Fatal(err)
	}
	enrollment := httptest.NewServer(fleet)
	t.Cleanup(enrollment.Close)

	request := []byte(`{"schema_version":1,"enrollment_attempt_id":"attempt-1","agent_id":"agent-1","invitation_token":"invitation-token"}`)
	status, body := exchange(t, enrollment.URL, request)
	if status != http.StatusCreated {
		t.Fatalf("enrollment status = %d; want %d", status, http.StatusCreated)
	}
	var credential struct {
		CredentialID string `json:"credential_id"`
		Credential   string `json:"credential"`
	}
	if err := json.Unmarshal(body, &credential); err != nil {
		t.Fatalf("decode enrollment credential: %v", err)
	}

	server, err := startOpAMP(fleet)
	if err != nil {
		t.Fatalf("start provisional OpAMP server: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.stop(ctx); err != nil {
			t.Errorf("stop provisional OpAMP server: %v", err)
		}
	})

	instanceUID, err := uuid.NewRandom()
	if err != nil {
		t.Fatal(err)
	}
	connected := make(chan struct{}, 1)
	client := opampclient.NewWebSocket(nil)
	if err := client.SetAgentDescription(&protobufs.AgentDescription{
		IdentifyingAttributes: []*protobufs.KeyValue{{
			Key:   "service.name",
			Value: &protobufs.AnyValue{Value: &protobufs.AnyValue_StringValue{StringValue: "mockfleet.test"}},
		}},
	}); err != nil {
		t.Fatalf("set OpAMP agent description: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := client.Start(ctx, clienttypes.StartSettings{
		OpAMPServerURL: server.urlString(),
		Header:         http.Header{"Authorization": []string{"Bearer " + credential.Credential}},
		InstanceUid:    clienttypes.InstanceUid(instanceUID),
		Callbacks: clienttypes.Callbacks{
			OnConnect: func(context.Context) {
				select {
				case connected <- struct{}{}:
				default:
				}
			},
		},
	}); err != nil {
		t.Fatalf("start authenticated OpAMP client: %v", err)
	}
	clientStopped := false
	t.Cleanup(func() {
		if clientStopped {
			return
		}
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		if err := client.Stop(stopCtx); err != nil {
			t.Errorf("stop authenticated OpAMP client: %v", err)
		}
	})
	select {
	case <-connected:
	case <-ctx.Done():
		t.Fatal("fixture-issued credential did not establish an OpAMP session")
	}

	if err := fleet.RevokeCredential(credential.CredentialID); err != nil {
		t.Fatalf("revoke fixture-issued credential: %v", err)
	}
	select {
	case <-server.sessionClosed():
	case <-ctx.Done():
		t.Fatal("revocation did not close the active OpAMP session")
	}
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := client.Stop(stopCtx); err != nil {
		stopCancel()
		t.Fatalf("stop revoked OpAMP client before explicit reconnect check: %v", err)
	}
	stopCancel()
	clientStopped = true

	reconnectURL := "http" + strings.TrimPrefix(server.urlString(), "ws")
	reconnect, err := http.NewRequestWithContext(ctx, http.MethodGet, reconnectURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	reconnect.Header.Set("Authorization", "Bearer "+credential.Credential)
	response, err := http.DefaultClient.Do(reconnect)
	if err != nil {
		t.Fatalf("attempt OpAMP reconnect with revoked credential: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked credential reconnect status = %d; want %d", response.StatusCode, http.StatusUnauthorized)
	}
	if got := server.rejectedConnections(); got != 1 {
		t.Fatalf("rejected OpAMP connections = %d; want 1", got)
	}
}
