package mockfleet

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"

	opampserver "github.com/open-telemetry/opamp-go/server"
	opamptypes "github.com/open-telemetry/opamp-go/server/types"
)

const exchangePath = "/v1/enrollments/exchange"

// Server is an in-memory fixture for the illustrative Fleet enrollment
// exchange. Its routes and response details remain provisional.
type Server struct {
	mu          sync.Mutex
	invitations map[string]invitation
	attempts    map[string]exchangeRecord
	credentials map[string]*issuedCredential
}

type invitation struct {
	nodeID string
}

type exchangeRecord struct {
	request exchangeRequest
	result  exchangeResult
}

type issuedCredential struct {
	secret   string
	revoked  bool
	sessions map[opamptypes.Connection]struct{}
}

type exchangeRequest struct {
	SchemaVersion int              `json:"schema_version"`
	AttemptID     string           `json:"enrollment_attempt_id"`
	AgentID       string           `json:"agent_id"`
	Token         string           `json:"invitation_token"`
	Evidence      identityEvidence `json:"identity_evidence"`
}

type identityEvidence struct {
	HostID          string `json:"host_id"`
	ComputeType     string `json:"compute_type"`
	Orchestrator    string `json:"orchestrator"`
	DeploymentScope string `json:"deployment_scope"`
}

type exchangeResult struct {
	SchemaVersion int    `json:"schema_version"`
	AgentID       string `json:"agent_id"`
	NodeID        string `json:"node_id"`
	CredentialID  string `json:"credential_id"`
	Credential    string `json:"credential"`
	OpAMPEndpoint string `json:"opamp_endpoint"`
}

// New returns an empty provisional fixture. Tests seed invitations directly so
// this package does not invent an unapproved Core-to-Fleet HTTP endpoint.
func New() *Server {
	return &Server{
		invitations: make(map[string]invitation),
		attempts:    make(map[string]exchangeRecord),
		credentials: make(map[string]*issuedCredential),
	}
}

// SeedInvitation adds a test invitation through the fixture boundary. It does
// not model Core authentication, invitation authorization, or durable storage.
func (s *Server) SeedInvitation(token, nodeID string) error {
	if s == nil || strings.TrimSpace(token) == "" || strings.TrimSpace(nodeID) == "" {
		return errors.New("mock Fleet invitation requires a token and node ID")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.invitations[token]; exists {
		return errors.New("mock Fleet invitation token already exists")
	}
	s.invitations[token] = invitation{nodeID: nodeID}
	return nil
}

// ServeHTTP implements the single illustrative enrollment exchange route.
func (s *Server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path != exchangePath {
		http.NotFound(writer, request)
		return
	}
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writeError(writer, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	if s == nil {
		writeError(writer, http.StatusInternalServerError, "fixture_unavailable")
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, 64*1024)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var exchange exchangeRequest
	if err := decoder.Decode(&exchange); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_request")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(writer, http.StatusBadRequest, "invalid_request")
		return
	}
	if exchange.SchemaVersion != 1 || strings.TrimSpace(exchange.AttemptID) == "" ||
		strings.TrimSpace(exchange.AgentID) == "" || strings.TrimSpace(exchange.Token) == "" {
		writeError(writer, http.StatusBadRequest, "invalid_request")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if previous, exists := s.attempts[exchange.AttemptID]; exists {
		if previous.request != exchange {
			writeError(writer, http.StatusConflict, "attempt_conflict")
			return
		}
		writeResult(writer, http.StatusCreated, previous.result)
		return
	}
	invited, exists := s.invitations[exchange.Token]
	if !exists {
		writeError(writer, http.StatusConflict, "invitation_unavailable")
		return
	}
	for _, previous := range s.attempts {
		if previous.request.Token == exchange.Token {
			writeError(writer, http.StatusConflict, "invitation_unavailable")
			return
		}
	}
	result := exchangeResult{
		SchemaVersion: 1,
		AgentID:       exchange.AgentID,
		NodeID:        invited.nodeID,
		CredentialID:  "mock-credential-id-" + exchange.AttemptID,
		Credential:    "mock-credential-" + exchange.AttemptID,
		OpAMPEndpoint: "wss://fleet.test.invalid/opamp",
	}
	s.attempts[exchange.AttemptID] = exchangeRecord{request: exchange, result: result}
	s.credentials[result.CredentialID] = &issuedCredential{
		secret:   result.Credential,
		sessions: make(map[opamptypes.Connection]struct{}),
	}
	writeResult(writer, http.StatusCreated, result)
}

func (s *Server) revokeCredential(credentialID string) error {
	if s == nil || strings.TrimSpace(credentialID) == "" {
		return errors.New("mock Fleet credential ID is required")
	}
	s.mu.Lock()
	credential, exists := s.credentials[credentialID]
	if !exists {
		s.mu.Unlock()
		return errors.New("mock Fleet credential does not exist")
	}
	credential.revoked = true
	sessions := make([]opamptypes.Connection, 0, len(credential.sessions))
	for session := range credential.sessions {
		sessions = append(sessions, session)
	}
	s.mu.Unlock()

	var disconnectErrors []error
	for _, session := range sessions {
		if err := session.Disconnect(); err != nil {
			disconnectErrors = append(disconnectErrors, err)
		}
	}
	return errors.Join(disconnectErrors...)
}

func (s *Server) acceptsCredential(secret string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for credentialID, credential := range s.credentials {
		if credential.secret == secret && !credential.revoked {
			return credentialID, true
		}
	}
	return "", false
}

func (s *Server) addSession(credentialID string, session opamptypes.Connection) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	credential, exists := s.credentials[credentialID]
	if !exists || credential.revoked {
		return false
	}
	credential.sessions[session] = struct{}{}
	return true
}

func (s *Server) removeSession(credentialID string, session opamptypes.Connection) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if credential, exists := s.credentials[credentialID]; exists {
		delete(credential.sessions, session)
	}
}

type opampFixture struct {
	server   opampserver.OpAMPServer
	url      string
	closed   chan struct{}
	rejected chan struct{}
}

func startOpAMP(fleet *Server) (*opampFixture, error) {
	if fleet == nil {
		return nil, errors.New("mock Fleet fixture is required")
	}
	fixture := &opampFixture{
		server:   opampserver.New(nil),
		closed:   make(chan struct{}, 8),
		rejected: make(chan struct{}, 8),
	}
	err := fixture.server.Start(opampserver.StartSettings{
		ListenEndpoint: "127.0.0.1:0",
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
					credentialID, accepted := fleet.acceptsCredential(strings.TrimPrefix(authorization, bearerPrefix))
					if !accepted {
						fixture.recordRejected()
						return opamptypes.ConnectionResponse{Accept: false, HTTPStatusCode: http.StatusUnauthorized}
					}
					return opamptypes.ConnectionResponse{
						Accept: true,
						ConnectionCallbacks: opamptypes.ConnectionCallbacks{
							OnConnected: func(_ context.Context, connection opamptypes.Connection) {
								if !fleet.addSession(credentialID, connection) {
									_ = connection.Disconnect()
								}
							},
							OnConnectionClose: func(connection opamptypes.Connection) {
								fleet.removeSession(credentialID, connection)
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

func (f *opampFixture) urlString() string {
	return f.url
}

func (f *opampFixture) stop(ctx context.Context) error {
	return f.server.Stop(ctx)
}

func (f *opampFixture) sessionClosed() <-chan struct{} {
	return f.closed
}

func (f *opampFixture) rejectedConnections() int {
	return len(f.rejected)
}

func (f *opampFixture) recordRejected() {
	select {
	case f.rejected <- struct{}{}:
	default:
	}
}

func writeResult(writer http.ResponseWriter, status int, result exchangeResult) {
	payload, err := json.Marshal(result)
	if err != nil {
		http.Error(writer, "mock Fleet response encoding failed", http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_, _ = writer.Write(append(payload, '\n'))
}

func writeError(writer http.ResponseWriter, status int, code string) {
	payload, err := json.Marshal(struct {
		Error string `json:"error"`
	}{Error: code})
	if err != nil {
		http.Error(writer, "mock Fleet error encoding failed", http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_, _ = writer.Write(append(payload, '\n'))
}
