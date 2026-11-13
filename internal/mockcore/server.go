package mockcore

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/coreapi"
	"github.com/ar-imms/telemetry-agent/internal/coreconfig"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

const managementPrefix = "/api/v1/agent-management/"

// ProvisionedInstallation is test-only seed data. The bearer value is hashed
// before it is retained by the Mock Core.
type ProvisionedInstallation struct {
	AccessToken    string `json:"-"`
	InstallationID string
	NodeID         string
	InstanceUID    string
	Policy         coreapi.DesiredGatewayPolicy
}

// StatusObservation contains a status accepted for one authenticated target.
type StatusObservation struct {
	InstallationID string
	Report         coreapi.SupervisorStatusReport
}

// Server serves the local Agent management contract from in-memory state.
type Server struct {
	mu            sync.RWMutex
	installations map[[32]byte]*ProvisionedInstallation
	statuses      []StatusObservation
	statusKeys    map[statusKey]struct{}
	publications  map[publicationKey]coreapi.ConfigurationPublication
	statePath     string
}

type statusKey struct {
	InstallationID string
	Report         coreapi.SupervisorStatusReport
}

type publicationKey struct {
	InstallationID string
	InstanceUID    string
	PolicyRevision int64
	ConfigHash     string
}

type persistentSnapshot struct {
	Installations []persistentInstallation           `json:"installations"`
	Publications  []coreapi.ConfigurationPublication `json:"publications"`
	Statuses      []StatusObservation                `json:"statuses"`
}

type persistentInstallation struct {
	TokenHash string                  `json:"token_hash"`
	Value     ProvisionedInstallation `json:"value"`
}

// New validates test seed data and returns a replaceable HTTP handler.
func New(seed ProvisionedInstallation) (*Server, error) {
	if strings.TrimSpace(seed.AccessToken) == "" || strings.TrimSpace(seed.AccessToken) != seed.AccessToken {
		return nil, fmt.Errorf("mock Core access token is required")
	}
	if _, err := identity.ParseInstallationID(seed.InstallationID); err != nil {
		return nil, fmt.Errorf("mock Core installation ID is invalid")
	}
	if _, err := identity.ParseInstallationID(seed.NodeID); err != nil {
		return nil, fmt.Errorf("mock Core Node ID is invalid")
	}
	if seed.InstanceUID != "" {
		if _, err := identity.ParseInstallationID(seed.InstanceUID); err != nil {
			return nil, fmt.Errorf("mock Core OpAMP instance UID is invalid")
		}
	}
	if seed.Policy.SchemaVersion != "ar-imms/gateway-policy/v1" || seed.Policy.Revision < 1 || coreconfig.ValidateGatewayEndpoint(seed.Policy.GatewayEndpoint) != nil {
		return nil, fmt.Errorf("mock Core Gateway policy is invalid")
	}
	tokenHash := sha256.Sum256([]byte(seed.AccessToken))
	seed.AccessToken = ""
	return &Server{installations: map[[32]byte]*ProvisionedInstallation{tokenHash: &seed}, statusKeys: make(map[statusKey]struct{}), publications: make(map[publicationKey]coreapi.ConfigurationPublication)}, nil
}

// NewPersistent opens a Mock Core fixture that retains bindings, policies,
// publications, and statuses across server restarts without storing bearer values.
func NewPersistent(seed ProvisionedInstallation, statePath string) (*Server, error) {
	if strings.TrimSpace(statePath) == "" {
		return nil, fmt.Errorf("persistent Mock Core state path is required")
	}
	server, err := New(seed)
	if err != nil {
		return nil, err
	}
	server.statePath = filepath.Clean(statePath)
	content, err := os.ReadFile(server.statePath)
	if errors.Is(err, os.ErrNotExist) {
		server.mu.Lock()
		err = server.persistLocked()
		server.mu.Unlock()
		if err != nil {
			return nil, fmt.Errorf("initialize persistent Mock Core state: %w", err)
		}
		return server, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read persistent Mock Core state: %w", err)
	}
	if len(content) > 4<<20 {
		return nil, fmt.Errorf("persistent Mock Core state exceeds size limit")
	}
	var snapshot persistentSnapshot
	if err := json.Unmarshal(content, &snapshot); err != nil {
		return nil, fmt.Errorf("decode persistent Mock Core state: %w", err)
	}
	if err := server.restore(snapshot); err != nil {
		return nil, err
	}
	return server, nil
}

func (s *Server) restore(snapshot persistentSnapshot) error {
	s.installations = make(map[[32]byte]*ProvisionedInstallation, len(snapshot.Installations))
	s.publications = make(map[publicationKey]coreapi.ConfigurationPublication, len(snapshot.Publications))
	s.statuses = append([]StatusObservation(nil), snapshot.Statuses...)
	s.statusKeys = make(map[statusKey]struct{}, len(snapshot.Statuses))
	for _, entry := range snapshot.Installations {
		hash, err := hex.DecodeString(entry.TokenHash)
		if err != nil || len(hash) != 32 || invalidID(entry.Value.InstallationID) || invalidID(entry.Value.NodeID) {
			return fmt.Errorf("persistent Mock Core installation record is invalid")
		}
		var tokenHash [32]byte
		copy(tokenHash[:], hash)
		value := entry.Value
		if value.AccessToken != "" || value.Policy.SchemaVersion != "ar-imms/gateway-policy/v1" || value.Policy.Revision < 1 || coreconfig.ValidateGatewayEndpoint(value.Policy.GatewayEndpoint) != nil {
			return fmt.Errorf("persistent Mock Core installation policy is invalid")
		}
		s.installations[tokenHash] = &value
	}
	for _, publication := range snapshot.Publications {
		if invalidID(publication.InstanceUID) || invalidID(publication.InstallationID) || publication.PolicyRevision < 1 || publication.PolicySchemaVersion != "ar-imms/gateway-policy/v1" || !validHash(publication.ConfigHash) || publication.PublishedAt.IsZero() {
			return fmt.Errorf("persistent Mock Core publication record is invalid")
		}
		key := publicationKey{InstallationID: publication.InstallationID, InstanceUID: publication.InstanceUID, PolicyRevision: publication.PolicyRevision, ConfigHash: publication.ConfigHash}
		s.publications[key] = publication
	}
	for _, observation := range s.statuses {
		s.statusKeys[statusKey{InstallationID: observation.InstallationID, Report: observation.Report}] = struct{}{}
	}
	return nil
}

func (s *Server) persistLocked() error {
	if s.statePath == "" {
		return nil
	}
	snapshot := persistentSnapshot{Statuses: append([]StatusObservation(nil), s.statuses...)}
	for tokenHash, installation := range s.installations {
		snapshot.Installations = append(snapshot.Installations, persistentInstallation{TokenHash: hex.EncodeToString(tokenHash[:]), Value: *installation})
	}
	sort.Slice(snapshot.Installations, func(i, j int) bool { return snapshot.Installations[i].TokenHash < snapshot.Installations[j].TokenHash })
	for _, publication := range s.publications {
		snapshot.Publications = append(snapshot.Publications, publication)
	}
	sort.Slice(snapshot.Publications, func(i, j int) bool {
		left, right := snapshot.Publications[i], snapshot.Publications[j]
		if left.InstallationID != right.InstallationID {
			return left.InstallationID < right.InstallationID
		}
		if left.InstanceUID != right.InstanceUID {
			return left.InstanceUID < right.InstanceUID
		}
		if left.PolicyRevision != right.PolicyRevision {
			return left.PolicyRevision < right.PolicyRevision
		}
		return left.ConfigHash < right.ConfigHash
	})
	content, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode persistent Mock Core state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.statePath), 0o700); err != nil {
		return fmt.Errorf("create persistent Mock Core state directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(s.statePath), ".mock-core-state-*")
	if err != nil {
		return fmt.Errorf("create persistent Mock Core state file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("protect persistent Mock Core state file: %w", err)
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return fmt.Errorf("write persistent Mock Core state: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync persistent Mock Core state: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close persistent Mock Core state: %w", err)
	}
	if err := replacePersistentState(temporaryPath, s.statePath); err != nil {
		return fmt.Errorf("activate persistent Mock Core state: %w", err)
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	directory, err := os.Open(filepath.Dir(s.statePath))
	if err != nil {
		return fmt.Errorf("open persistent Mock Core state directory: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil && runtime.GOOS != "windows" {
		return fmt.Errorf("sync persistent Mock Core state directory: %w", err)
	}
	return nil
}

// ServeHTTP dispatches only the versioned Agent management operations.
func (s *Server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method == http.MethodGet && request.URL.Path == managementPrefix+"session" {
		s.getSession(writer, request)
		return
	}
	if request.Method == http.MethodPut && request.URL.Path == managementPrefix+"opamp-instance" {
		s.bindInstance(writer, request)
		return
	}
	if request.Method == http.MethodPut && request.URL.Path == managementPrefix+"publications" {
		s.recordPublication(writer, request)
		return
	}
	if request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, managementPrefix+"publications/") {
		s.getPublication(writer, request)
		return
	}
	if request.Method == http.MethodPut && request.URL.Path == managementPrefix+"status" {
		s.reportStatus(writer, request)
		return
	}
	s.writeProblem(writer, request, http.StatusNotFound, "NOT_FOUND", "The requested Agent management operation does not exist.")
}

func (s *Server) recordPublication(writer http.ResponseWriter, request *http.Request) {
	_, tokenHash, ok := s.authenticate(writer, request)
	if !ok {
		return
	}
	var publication coreapi.PublicationRequest
	if !decodeBody(writer, request, &publication) || invalidID(publication.InstanceUID) || publication.PolicySchemaVersion != "ar-imms/gateway-policy/v1" || publication.PolicyRevision < 1 || !validHash(publication.ConfigHash) {
		s.writeProblem(writer, request, http.StatusBadRequest, "VALIDATION_ERROR", "The configuration publication is invalid.")
		return
	}
	s.mu.Lock()
	installation := s.installations[tokenHash]
	if installation == nil {
		s.mu.Unlock()
		s.writeProblem(writer, request, http.StatusUnauthorized, "UNAUTHORIZED", "A valid Agent bearer credential is required.")
		return
	}
	if installation.InstanceUID != publication.InstanceUID {
		s.mu.Unlock()
		s.writeProblem(writer, request, http.StatusForbidden, "OPAMP_INSTANCE_MISMATCH", "The publication does not match the authenticated installation binding.")
		return
	}
	if installation.Policy.SchemaVersion != publication.PolicySchemaVersion || installation.Policy.Revision != publication.PolicyRevision {
		s.mu.Unlock()
		s.writeProblem(writer, request, http.StatusConflict, "STALE_POLICY_REVISION", "The publication does not match the current desired policy revision.")
		return
	}
	key := publicationKey{InstallationID: installation.InstallationID, InstanceUID: publication.InstanceUID, PolicyRevision: publication.PolicyRevision, ConfigHash: publication.ConfigHash}
	stored := s.publications[key]
	if stored.PublishedAt.IsZero() {
		stored = coreapi.ConfigurationPublication{PublicationRequest: publication, InstallationID: installation.InstallationID, PublishedAt: time.Now().UTC()}
		s.publications[key] = stored
		if err := s.persistLocked(); err != nil {
			delete(s.publications, key)
			s.mu.Unlock()
			s.writeProblem(writer, request, http.StatusInternalServerError, "PERSISTENCE_FAILED", "The configuration publication could not be committed.")
			return
		}
	}
	s.mu.Unlock()
	s.writeJSON(writer, request, http.StatusOK, map[string]any{"data": stored})
}

func (s *Server) getPublication(writer http.ResponseWriter, request *http.Request) {
	_, tokenHash, ok := s.authenticate(writer, request)
	if !ok {
		return
	}
	hash := strings.TrimPrefix(request.URL.Path, managementPrefix+"publications/")
	if !validHash(hash) || strings.Contains(hash, "/") {
		s.writeProblem(writer, request, http.StatusBadRequest, "VALIDATION_ERROR", "The configuration hash is invalid.")
		return
	}
	s.mu.RLock()
	installation := s.installations[tokenHash]
	if installation == nil {
		s.mu.RUnlock()
		s.writeProblem(writer, request, http.StatusUnauthorized, "UNAUTHORIZED", "A valid Agent bearer credential is required.")
		return
	}
	var resolved coreapi.ConfigurationPublication
	for key, publication := range s.publications {
		if key.InstallationID == installation.InstallationID && key.InstanceUID == installation.InstanceUID && key.ConfigHash == hash && key.PolicyRevision >= resolved.PolicyRevision {
			resolved = publication
		}
	}
	s.mu.RUnlock()
	if resolved.PublishedAt.IsZero() {
		s.writeProblem(writer, request, http.StatusNotFound, "PUBLICATION_NOT_FOUND", "No configuration publication matches this hash and binding.")
		return
	}
	s.writeJSON(writer, request, http.StatusOK, map[string]any{"data": resolved})
}

// SetPolicy advances the target's immutable desired policy in the test server.
func (s *Server) SetPolicy(installationID string, policy coreapi.DesiredGatewayPolicy) error {
	if s == nil || policy.SchemaVersion != "ar-imms/gateway-policy/v1" || policy.Revision < 1 || coreconfig.ValidateGatewayEndpoint(policy.GatewayEndpoint) != nil {
		return fmt.Errorf("mock Core Gateway policy is invalid")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	installation := s.findInstallationByID(installationID)
	if installation == nil {
		return fmt.Errorf("mock Core installation does not exist")
	}
	if policy.Revision <= installation.Policy.Revision {
		return fmt.Errorf("mock Core policy revision must increase")
	}
	previous := installation.Policy
	installation.Policy = policy
	if err := s.persistLocked(); err != nil {
		installation.Policy = previous
		return err
	}
	return nil
}

// Revoke removes the test credential so later requests receive 401.
func (s *Server) Revoke(accessToken string) error {
	if s == nil {
		return nil
	}
	hash := sha256.Sum256([]byte(accessToken))
	s.mu.Lock()
	previous, exists := s.installations[hash]
	delete(s.installations, hash)
	err := s.persistLocked()
	if err != nil && exists {
		s.installations[hash] = previous
	}
	s.mu.Unlock()
	return err
}

// Statuses returns a defensive copy of accepted observations for assertions.
func (s *Server) Statuses() []StatusObservation {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]StatusObservation(nil), s.statuses...)
}

func (s *Server) getSession(writer http.ResponseWriter, request *http.Request) {
	installation, _, ok := s.authenticate(writer, request)
	if !ok {
		return
	}
	var instanceUID any
	if installation.InstanceUID != "" {
		instanceUID = installation.InstanceUID
	}
	s.writeJSON(writer, request, http.StatusOK, map[string]any{"data": map[string]any{
		"installation_id":    installation.InstallationID,
		"node_id":            installation.NodeID,
		"opamp_instance_uid": instanceUID,
		"desired_policy":     installation.Policy,
	}})
}

func (s *Server) bindInstance(writer http.ResponseWriter, request *http.Request) {
	_, tokenHash, ok := s.authenticate(writer, request)
	if !ok {
		return
	}
	var body struct {
		InstanceUID string `json:"instance_uid"`
	}
	if !decodeBody(writer, request, &body) || invalidID(body.InstanceUID) {
		s.writeProblem(writer, request, http.StatusBadRequest, "VALIDATION_ERROR", "The OpAMP instance UID is invalid.")
		return
	}
	s.mu.Lock()
	installation := s.installations[tokenHash]
	if installation == nil {
		s.mu.Unlock()
		s.writeProblem(writer, request, http.StatusUnauthorized, "UNAUTHORIZED", "A valid Agent bearer credential is required.")
		return
	}
	previousInstanceUID := installation.InstanceUID
	if installation.InstanceUID == "" {
		installation.InstanceUID = body.InstanceUID
	}
	if installation.InstanceUID != body.InstanceUID {
		s.mu.Unlock()
		s.writeProblem(writer, request, http.StatusConflict, "OPAMP_INSTANCE_CONFLICT", "The authenticated installation is already bound to another OpAMP instance.")
		return
	}
	if err := s.persistLocked(); err != nil {
		installation.InstanceUID = previousInstanceUID
		s.mu.Unlock()
		s.writeProblem(writer, request, http.StatusInternalServerError, "PERSISTENCE_FAILED", "The OpAMP binding could not be committed.")
		return
	}
	s.mu.Unlock()
	s.writeJSON(writer, request, http.StatusOK, map[string]any{"data": map[string]string{"instance_uid": body.InstanceUID}})
}

func (s *Server) reportStatus(writer http.ResponseWriter, request *http.Request) {
	_, tokenHash, ok := s.authenticate(writer, request)
	if !ok {
		return
	}
	var report coreapi.SupervisorStatusReport
	if !decodeBody(writer, request, &report) || !validStatusReport(report) {
		s.writeProblem(writer, request, http.StatusBadRequest, "VALIDATION_ERROR", "The Supervisor status report is invalid.")
		return
	}
	s.mu.Lock()
	installation := s.installations[tokenHash]
	if installation == nil {
		s.mu.Unlock()
		s.writeProblem(writer, request, http.StatusUnauthorized, "UNAUTHORIZED", "A valid Agent bearer credential is required.")
		return
	}
	if installation.InstanceUID == "" || report.InstanceUID != installation.InstanceUID {
		s.mu.Unlock()
		s.writeProblem(writer, request, http.StatusForbidden, "OPAMP_INSTANCE_MISMATCH", "The status report does not match the authenticated installation binding.")
		return
	}
	if report.DesiredRevision != installation.Policy.Revision {
		s.mu.Unlock()
		s.writeProblem(writer, request, http.StatusConflict, "STALE_POLICY_REVISION", "The status report does not target the current desired policy revision.")
		return
	}
	publicationKey := publicationKey{InstallationID: installation.InstallationID, InstanceUID: report.InstanceUID, PolicyRevision: report.PublicationRevision, ConfigHash: report.ConfigHash}
	if _, exists := s.publications[publicationKey]; !exists {
		s.mu.Unlock()
		s.writeProblem(writer, request, http.StatusNotFound, "PUBLICATION_NOT_FOUND", "The status does not match a configuration publication.")
		return
	}
	if report.Status != "APPLYING" && report.Status != "APPLIED" && report.Status != "FAILED" {
		s.mu.Unlock()
		s.writeProblem(writer, request, http.StatusBadRequest, "VALIDATION_ERROR", "The Supervisor status is unsupported.")
		return
	}
	key := statusKey{InstallationID: installation.InstallationID, Report: report}
	if _, exists := s.statusKeys[key]; !exists {
		s.statusKeys[key] = struct{}{}
		s.statuses = append(s.statuses, StatusObservation{InstallationID: installation.InstallationID, Report: report})
		if err := s.persistLocked(); err != nil {
			delete(s.statusKeys, key)
			s.statuses = s.statuses[:len(s.statuses)-1]
			s.mu.Unlock()
			s.writeProblem(writer, request, http.StatusInternalServerError, "PERSISTENCE_FAILED", "The Supervisor status could not be committed.")
			return
		}
	}
	s.mu.Unlock()
	writer.WriteHeader(http.StatusNoContent)
}

func (s *Server) authenticate(writer http.ResponseWriter, request *http.Request) (ProvisionedInstallation, [32]byte, bool) {
	parts := strings.SplitN(request.Header.Get("Authorization"), " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" || strings.ContainsAny(parts[1], " \t\r\n") {
		s.writeProblem(writer, request, http.StatusUnauthorized, "UNAUTHORIZED", "A valid Agent bearer credential is required.")
		return ProvisionedInstallation{}, [32]byte{}, false
	}
	hash := sha256.Sum256([]byte(parts[1]))
	s.mu.RLock()
	defer s.mu.RUnlock()
	for candidate, installation := range s.installations {
		if subtle.ConstantTimeCompare(hash[:], candidate[:]) == 1 {
			copy := *installation
			return copy, candidate, true
		}
	}
	s.writeProblem(writer, request, http.StatusUnauthorized, "UNAUTHORIZED", "A valid Agent bearer credential is required.")
	return ProvisionedInstallation{}, [32]byte{}, false
}

func (s *Server) findInstallationByID(installationID string) *ProvisionedInstallation {
	for _, installation := range s.installations {
		if installation.InstallationID == installationID {
			return installation
		}
	}
	return nil
}

func decodeBody(writer http.ResponseWriter, request *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 16*1024))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target) == nil
}

func (s *Server) writeJSON(writer http.ResponseWriter, request *http.Request, status int, body map[string]any) {
	body["meta"] = map[string]string{"request_id": requestID()}
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(body)
}

func (s *Server) writeProblem(writer http.ResponseWriter, request *http.Request, status int, code, detail string) {
	writer.Header().Set("Content-Type", "application/problem+json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]any{"type": "about:blank", "title": http.StatusText(status), "status": status, "detail": detail, "code": code, "request_id": requestID()})
}

func requestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "mockcore-unavailable"
	}
	return hex.EncodeToString(value[:])
}

func invalidID(value string) bool {
	_, err := identity.ParseInstallationID(value)
	return err != nil
}

func validStatusReport(report coreapi.SupervisorStatusReport) bool {
	if invalidID(report.InstanceUID) || report.DesiredRevision < 1 || report.PublicationRevision < 1 || report.PublicationRevision > report.DesiredRevision || report.AppliedRevision < 0 || report.AppliedRevision > report.DesiredRevision || !validHash(report.ConfigHash) || (report.Status == "APPLIED" && report.AppliedRevision != report.PublicationRevision) || (report.Status != "APPLIED" && report.AppliedRevision != 0) || len(report.Message) > 512 || strings.ContainsAny(report.Message, "\r\n\x00") {
		return false
	}
	if report.Status != "APPLYING" && report.Status != "APPLIED" && report.Status != "FAILED" {
		return false
	}
	return true
}

func validHash(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}
