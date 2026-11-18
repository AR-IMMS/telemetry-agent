package mockfleet

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
)

const exchangePath = "/v1/enrollments/exchange"

// Server is an in-memory fixture for the illustrative Fleet enrollment
// exchange. Its routes and response details remain provisional.
type Server struct {
	mu          sync.Mutex
	invitations map[string]invitation
	attempts    map[string]exchangeRecord
}

type invitation struct {
	nodeID string
}

type exchangeRecord struct {
	request exchangeRequest
	result  exchangeResult
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
	writeResult(writer, http.StatusCreated, result)
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
