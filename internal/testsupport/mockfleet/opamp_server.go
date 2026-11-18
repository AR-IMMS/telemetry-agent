package mockfleet

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/open-telemetry/opamp-go/protobufs"
	opampserver "github.com/open-telemetry/opamp-go/server"
	opamptypes "github.com/open-telemetry/opamp-go/server/types"
)

// OpAMPServer is an authenticated in-memory OpAMP fixture for tests. It is not
// an AR-IMMS Fleet server and persists no desired state.
type OpAMPServer struct {
	server   opampserver.OpAMPServer
	url      string
	closed   chan struct{}
	rejected chan struct{}
	mu       sync.Mutex
	sentHash map[opamptypes.Connection][]byte
}

// StartOpAMPServer starts a loopback OpAMP endpoint for authenticated fixture
// credentials. Config and status messages use the standard OpAMP protobuf.
func (s *Server) StartOpAMPServer() (*OpAMPServer, error) {
	return s.StartOpAMPServerAt("127.0.0.1:0")
}

// StartOpAMPServerAt starts the authenticated fixture on a specific loopback
// endpoint. A fixed endpoint lets E2E tests delay server availability until a
// managed Supervisor has completed its offline startup path.
func (s *Server) StartOpAMPServerAt(listenEndpoint string) (*OpAMPServer, error) {
	if s == nil {
		return nil, errors.New("mock Fleet fixture is required")
	}
	if strings.TrimSpace(listenEndpoint) == "" {
		return nil, errors.New("OpAMP fixture listen endpoint is required")
	}
	fixture := &OpAMPServer{
		server:   opampserver.New(nil),
		closed:   make(chan struct{}, 8),
		rejected: make(chan struct{}, 8),
		sentHash: make(map[opamptypes.Connection][]byte),
	}
	err := fixture.server.Start(opampserver.StartSettings{
		ListenEndpoint: listenEndpoint,
		ListenPath:     "/v1/opamp",
		Settings: opampserver.Settings{
			Callbacks: opamptypes.Callbacks{
				OnConnecting: func(request *http.Request) opamptypes.ConnectionResponse {
					const bearerPrefix = "Bearer "
					authorization := request.Header.Get("Authorization")
					if !strings.HasPrefix(authorization, bearerPrefix) {
						fixture.recordRejected()
						return opamptypes.ConnectionResponse{Accept: false, HTTPStatusCode: http.StatusUnauthorized}
					}
					credentialID, agentID, accepted := s.acceptsCredential(strings.TrimPrefix(authorization, bearerPrefix))
					if !accepted {
						fixture.recordRejected()
						return opamptypes.ConnectionResponse{Accept: false, HTTPStatusCode: http.StatusUnauthorized}
					}
					return opamptypes.ConnectionResponse{
						Accept: true,
						ConnectionCallbacks: opamptypes.ConnectionCallbacks{
							OnConnected: func(_ context.Context, connection opamptypes.Connection) {
								if !s.addSession(credentialID, connection) {
									_ = connection.Disconnect()
								}
							},
							OnMessage: func(_ context.Context, connection opamptypes.Connection, message *protobufs.AgentToServer) *protobufs.ServerToAgent {
								s.recordRemoteConfigStatus(agentID, message.GetRemoteConfigStatus())
								response := &protobufs.ServerToAgent{
									InstanceUid:  message.GetInstanceUid(),
									Capabilities: uint64(protobufs.ServerCapabilities_ServerCapabilities_OffersRemoteConfig),
								}
								config := s.desiredConfig(agentID)
								if len(config) == 0 {
									return response
								}
								hash := sha256.Sum256(config)
								fixture.mu.Lock()
								if bytes.Equal(fixture.sentHash[connection], hash[:]) {
									fixture.mu.Unlock()
									return response
								}
								fixture.sentHash[connection] = append([]byte(nil), hash[:]...)
								fixture.mu.Unlock()
								response.RemoteConfig = &protobufs.AgentRemoteConfig{
									Config: &protobufs.AgentConfigMap{ConfigMap: map[string]*protobufs.AgentConfigFile{
										"": {Body: config},
									}},
									ConfigHash: hash[:],
								}
								return response
							},
							OnConnectionClose: func(connection opamptypes.Connection) {
								s.removeSession(credentialID, connection)
								fixture.mu.Lock()
								delete(fixture.sentHash, connection)
								fixture.mu.Unlock()
								select {
								case fixture.closed <- struct{}{}:
								default:
								}
							},
						},
					}
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	fixture.url = "ws://" + fixture.server.Addr().String() + "/v1/opamp"
	return fixture, nil
}

func startOpAMP(fleet *Server) (*OpAMPServer, error) {
	return fleet.StartOpAMPServer()
}

// Endpoint returns the loopback WebSocket endpoint used by the test fixture.
func (f *OpAMPServer) Endpoint() string {
	return f.url
}

// Stop closes the listener and all current OpAMP connections.
func (f *OpAMPServer) Stop(ctx context.Context) error {
	return f.server.Stop(ctx)
}

func (f *OpAMPServer) urlString() string {
	return f.Endpoint()
}

func (f *OpAMPServer) stop(ctx context.Context) error {
	return f.Stop(ctx)
}

func (f *OpAMPServer) sessionClosed() <-chan struct{} {
	return f.closed
}

func (f *OpAMPServer) rejectedConnections() int {
	return len(f.rejected)
}

// RejectedConnectionCount reports failed fixture authentication attempts.
func (f *OpAMPServer) RejectedConnectionCount() int {
	return f.rejectedConnections()
}

func (f *OpAMPServer) recordRejected() {
	select {
	case f.rejected <- struct{}{}:
	default:
	}
}
