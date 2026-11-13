package opampserver

import (
	"context"
	"net"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/coreapi"
	"github.com/ar-imms/telemetry-agent/internal/mockcore"
	"github.com/open-telemetry/opamp-go/protobufs"
	opamptypes "github.com/open-telemetry/opamp-go/server/types"
)

func TestEstablishedSessionDisconnectsAfterMockCoreRevokesCredential(t *testing.T) {
	const token = "active-opamp-session-token"
	core, err := mockcore.New(mockcore.ProvisionedInstallation{
		AccessToken: token, InstallationID: "11111111-2222-4333-8444-555555555555",
		NodeID: "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee",
		Policy: coreapi.DesiredGatewayPolicy{SchemaVersion: "ar-imms/gateway-policy/v1", Revision: 1, GatewayEndpoint: "127.0.0.1:4317"},
	})
	if err != nil {
		t.Fatal(err)
	}
	coreHTTP := httptest.NewTLSServer(core)
	defer coreHTTP.Close()
	client, err := coreapi.NewAgentManagementClient(coreHTTP.URL, token, coreHTTP.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetSession(context.Background()); err != nil {
		t.Fatalf("establish authenticated Core session: %v", err)
	}
	server, err := New(Options{ListenEndpoint: "127.0.0.1:0", CoreClient: func(string) (coreapi.AgentManagementAPI, error) { return nil, nil }})
	if err != nil {
		t.Fatal(err)
	}
	connection := newTestOpAMPConnection(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.monitorSession(ctx, connection, client, time.Millisecond)
	if err := core.Revoke(token); err != nil {
		t.Fatalf("revoke established session bearer in Mock Core: %v", err)
	}
	select {
	case <-connection.disconnected:
	case <-time.After(time.Second):
		t.Fatal("revoked OpAMP session was not disconnected")
	}
}

type testOpAMPConnection struct {
	connection   net.Conn
	disconnected chan struct{}
	once         sync.Once
}

func newTestOpAMPConnection(t *testing.T) *testOpAMPConnection {
	t.Helper()
	left, right := net.Pipe()
	t.Cleanup(func() { _ = left.Close(); _ = right.Close() })
	return &testOpAMPConnection{connection: left, disconnected: make(chan struct{})}
}
func (c *testOpAMPConnection) Connection() net.Conn                               { return c.connection }
func (*testOpAMPConnection) Send(context.Context, *protobufs.ServerToAgent) error { return nil }
func (c *testOpAMPConnection) Disconnect() error {
	c.once.Do(func() { close(c.disconnected) })
	return c.connection.Close()
}

var _ opamptypes.Connection = (*testOpAMPConnection)(nil)
