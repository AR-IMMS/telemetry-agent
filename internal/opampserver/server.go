package opampserver

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/coreapi"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	"github.com/google/uuid"
	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/open-telemetry/opamp-go/server"
	opamptypes "github.com/open-telemetry/opamp-go/server/types"
)

// ManagementClientFactory constructs a Core client bound to the supplied
// existing enrollment bearer credential.
type ManagementClientFactory func(string) (coreapi.AgentManagementAPI, error)

// Options configures the native OpAMP endpoint and Core contract adapter.
type Options struct {
	ListenEndpoint string
	ListenPath     string
	TLSConfig      *tls.Config
	CoreClient     ManagementClientFactory
	Logger         *slog.Logger
}

// Server exposes authenticated native OpAMP while keeping Core policy outside
// the protocol adapter.
type Server struct {
	options Options
	server  server.OpAMPServer
	mu      sync.Mutex
	sent    map[string]map[string]int64
}

// New validates server dependencies and rejects non-loopback cleartext binds.
func New(options Options) (*Server, error) {
	host, _, err := net.SplitHostPort(options.ListenEndpoint)
	if err != nil || host == "" || options.CoreClient == nil {
		return nil, fmt.Errorf("OpAMP endpoint and Core management client factory are required")
	}
	if options.TLSConfig == nil && !isLoopback(host) {
		return nil, fmt.Errorf("cleartext OpAMP is allowed only on loopback")
	}
	if options.ListenPath == "" {
		options.ListenPath = "/v1/opamp"
	}
	if options.ListenPath[0] != '/' {
		return nil, fmt.Errorf("OpAMP listen path must be absolute")
	}
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	return &Server{options: options, server: server.New(nil), sent: make(map[string]map[string]int64)}, nil
}

// Start begins accepting authenticated OpAMP sessions.
func (s *Server) Start() error {
	if s == nil || s.server == nil {
		return fmt.Errorf("OpAMP server is required")
	}
	callbacks := opamptypes.Callbacks{OnConnecting: s.onConnecting}
	return s.server.Start(server.StartSettings{
		Settings: server.Settings{Callbacks: callbacks}, ListenEndpoint: s.options.ListenEndpoint,
		ListenPath: s.options.ListenPath, TLSConfig: s.options.TLSConfig,
	})
}

// Stop stops the OpAMP listener and waits for its connections to close.
func (s *Server) Stop(ctx context.Context) error {
	if s == nil || s.server == nil {
		return nil
	}
	return s.server.Stop(ctx)
}

// Addr returns the listener address after Start succeeds.
func (s *Server) Addr() net.Addr {
	if s == nil || s.server == nil {
		return nil
	}
	return s.server.Addr()
}

func (s *Server) onConnecting(request *http.Request) opamptypes.ConnectionResponse {
	token, ok := bearerToken(request.Header.Get("Authorization"))
	if !ok {
		return rejectConnection(http.StatusUnauthorized)
	}
	client, err := s.options.CoreClient(token)
	if err != nil {
		s.options.Logger.Warn("Core management client unavailable", "error", err)
		return rejectConnection(http.StatusServiceUnavailable)
	}
	if _, err := client.GetSession(request.Context()); err != nil {
		var apiError *coreapi.ControlAPIError
		if errors.As(err, &apiError) && (apiError.Status == http.StatusUnauthorized || apiError.Status == http.StatusForbidden) {
			return rejectConnection(http.StatusUnauthorized)
		}
		s.options.Logger.Warn("OpAMP installation authentication failed", "error", err)
		return rejectConnection(http.StatusServiceUnavailable)
	}
	return opamptypes.ConnectionResponse{Accept: true, ConnectionCallbacks: opamptypes.ConnectionCallbacks{
		OnMessage: func(ctx context.Context, _ opamptypes.Connection, message *protobufs.AgentToServer) *protobufs.ServerToAgent {
			return s.handleMessage(ctx, client, message)
		},
	}}
}

func (s *Server) handleMessage(ctx context.Context, client coreapi.AgentManagementAPI, message *protobufs.AgentToServer) *protobufs.ServerToAgent {
	if message == nil || len(message.GetInstanceUid()) != 16 {
		return &protobufs.ServerToAgent{}
	}
	uidBytes := append([]byte(nil), message.GetInstanceUid()...)
	parsedUID, err := uuid.FromBytes(uidBytes)
	if err != nil {
		return &protobufs.ServerToAgent{InstanceUid: uidBytes}
	}
	uid := parsedUID.String()
	if _, err := identity.ParseInstallationID(uid); err != nil {
		return &protobufs.ServerToAgent{InstanceUid: uidBytes}
	}
	response := &protobufs.ServerToAgent{
		InstanceUid:  uidBytes,
		Capabilities: uint64(protobufs.ServerCapabilities_ServerCapabilities_OffersRemoteConfig),
	}
	if message.GetCapabilities()&uint64(protobufs.AgentCapabilities_AgentCapabilities_AcceptsRemoteConfig) == 0 {
		return response
	}
	session, err := client.GetSession(ctx)
	if err != nil {
		s.options.Logger.Warn("OpAMP policy lookup failed", "error", err)
		return &protobufs.ServerToAgent{InstanceUid: uidBytes}
	}
	if session.OpAMPInstanceUID == "" {
		bound, bindErr := client.BindOpAMPInstance(ctx, uid)
		if bindErr != nil || bound != uid {
			s.options.Logger.Warn("OpAMP instance binding failed", "error", bindErr)
			return &protobufs.ServerToAgent{InstanceUid: uidBytes}
		}
		session.OpAMPInstanceUID = bound
	}
	if session.OpAMPInstanceUID != uid {
		s.options.Logger.Warn("OpAMP instance UID does not match authenticated installation")
		return &protobufs.ServerToAgent{InstanceUid: uidBytes}
	}
	s.reportRemoteStatus(ctx, client, uid, session.DesiredPolicy.Revision, message.GetRemoteConfigStatus())
	body, hash, err := RenderGatewayConfig(session.DesiredPolicy)
	if err != nil {
		s.options.Logger.Warn("Core Gateway policy is invalid", "error", err)
		return &protobufs.ServerToAgent{InstanceUid: uidBytes}
	}
	hashText := hex.EncodeToString(hash[:])
	if s.wasSent(uid, hashText) {
		return response
	}
	response.RemoteConfig = &protobufs.AgentRemoteConfig{
		Config:     &protobufs.AgentConfigMap{ConfigMap: map[string]*protobufs.AgentConfigFile{"": {Body: body}}},
		ConfigHash: hash[:],
	}
	s.markSent(uid, hashText, session.DesiredPolicy.Revision)
	return response
}

func (s *Server) reportRemoteStatus(ctx context.Context, client coreapi.AgentManagementAPI, uid string, desiredRevision int64, status *protobufs.RemoteConfigStatus) {
	if status == nil || len(status.GetLastRemoteConfigHash()) != 32 {
		return
	}
	configHash := hex.EncodeToString(status.GetLastRemoteConfigHash())
	statusName := ""
	switch status.GetStatus() {
	case protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLYING:
		statusName = "APPLYING"
	case protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED:
		statusName = "APPLIED"
	case protobufs.RemoteConfigStatuses_RemoteConfigStatuses_FAILED:
		statusName = "FAILED"
	default:
		return
	}
	report := coreapi.SupervisorStatusReport{InstanceUID: uid, DesiredRevision: desiredRevision, Status: statusName, ConfigHash: configHash}
	if statusName == "APPLIED" {
		report.AppliedRevision = s.revisionForHash(uid, configHash)
	}
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.ReportStatus(callCtx, report); err != nil {
		s.options.Logger.Warn("OpAMP status report to Core failed", "error", err)
	}
}

func (s *Server) wasSent(uid, hash string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, found := s.sent[uid][hash]
	return found
}

func (s *Server) markSent(uid, hash string, revision int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sent[uid] == nil {
		s.sent[uid] = make(map[string]int64)
	}
	s.sent[uid][hash] = revision
}

func (s *Server) revisionForHash(uid, hash string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sent[uid][hash]
}

func bearerToken(header string) (string, bool) {
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" || strings.TrimSpace(parts[1]) != parts[1] || strings.ContainsAny(parts[1], " \t\r\n") {
		return "", false
	}
	return parts[1], true
}

func rejectConnection(status int) opamptypes.ConnectionResponse {
	return opamptypes.ConnectionResponse{Accept: false, HTTPStatusCode: status, HTTPResponseHeader: map[string]string{"Cache-Control": "no-store"}}
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}
