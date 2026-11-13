package opampserver

import (
	"context"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/coreapi"
	"github.com/open-telemetry/opamp-go/protobufs"
	opamptypes "github.com/open-telemetry/opamp-go/server/types"
)

func TestEstablishedSessionDisconnectsAfterCoreRevokesCredential(t *testing.T) {
	server, err := New(Options{ListenEndpoint: "127.0.0.1:0", CoreClient: func(string) (coreapi.AgentManagementAPI, error) { return nil, nil }})
	if err != nil {
		t.Fatal(err)
	}
	client := &revokedSessionClient{}
	connection := newTestOpAMPConnection(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.monitorSession(ctx, connection, client, time.Millisecond)
	select {
	case <-connection.disconnected:
	case <-time.After(time.Second):
		t.Fatal("revoked OpAMP session was not disconnected")
	}
	if client.calls == 0 {
		t.Fatal("Core session was not revalidated")
	}
}

type revokedSessionClient struct{ calls int }

func (c *revokedSessionClient) GetSession(context.Context) (coreapi.AgentManagementSession, error) {
	c.calls++
	return coreapi.AgentManagementSession{}, &coreapi.ControlAPIError{Status: http.StatusUnauthorized, Code: "UNAUTHORIZED"}
}
func (*revokedSessionClient) BindOpAMPInstance(context.Context, string) (string, error) {
	return "", nil
}
func (*revokedSessionClient) RecordPublication(context.Context, coreapi.PublicationRequest) (coreapi.ConfigurationPublication, error) {
	return coreapi.ConfigurationPublication{}, nil
}
func (*revokedSessionClient) ResolvePublication(context.Context, string, string) (coreapi.ConfigurationPublication, error) {
	return coreapi.ConfigurationPublication{}, nil
}
func (*revokedSessionClient) ReportStatus(context.Context, coreapi.SupervisorStatusReport) error {
	return nil
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
var _ coreapi.AgentManagementAPI = (*revokedSessionClient)(nil)
