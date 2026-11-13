package opampserver

import (
	"bytes"
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

const (
	maxTrackedInstances                = 64
	maxConfigHashesPerTrackedInstance  = 32
	maxActiveSupervisorSessions        = 64
	defaultSessionRevalidationInterval = 30 * time.Second
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
	options        Options
	server         server.OpAMPServer
	mu             sync.Mutex
	sent           map[string]map[string]int64
	lastUsed       map[string]uint64
	sessionCancels map[opamptypes.Connection]context.CancelFunc
	sequence       uint64
}

type sessionConfigCache struct {
	mu     sync.Mutex
	hashes map[string]string
}

func (c *sessionConfigCache) wasSent(uid, hash string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hashes[uid] == hash
}

func (c *sessionConfigCache) markSent(uid, hash string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.hashes == nil {
		c.hashes = make(map[string]string, 1)
	}
	c.hashes[uid] = hash
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
	return &Server{
		options: options, server: server.New(nil), sent: make(map[string]map[string]int64),
		lastUsed: make(map[string]uint64), sessionCancels: make(map[opamptypes.Connection]context.CancelFunc),
	}, nil
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
	err := s.server.Stop(ctx)
	s.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(s.sessionCancels))
	for connection, cancel := range s.sessionCancels {
		cancels = append(cancels, cancel)
		delete(s.sessionCancels, connection)
	}
	s.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	return err
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
	sessionConfigs := &sessionConfigCache{}
	return opamptypes.ConnectionResponse{Accept: true, ConnectionCallbacks: opamptypes.ConnectionCallbacks{
		OnConnected: func(_ context.Context, connection opamptypes.Connection) {
			s.startSessionMonitor(connection, client)
		},
		OnConnectionClose: s.stopSessionMonitor,
		OnMessage: func(ctx context.Context, _ opamptypes.Connection, message *protobufs.AgentToServer) *protobufs.ServerToAgent {
			return s.handleMessage(ctx, client, message, sessionConfigs)
		},
	}}
}

func (s *Server) startSessionMonitor(connection opamptypes.Connection, client coreapi.AgentManagementAPI) {
	if connection == nil || client == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	if s.sessionCancels == nil {
		s.sessionCancels = make(map[opamptypes.Connection]context.CancelFunc)
	}
	if _, exists := s.sessionCancels[connection]; exists {
		s.mu.Unlock()
		cancel()
		return
	}
	if len(s.sessionCancels) >= maxActiveSupervisorSessions {
		s.mu.Unlock()
		cancel()
		logger := s.options.Logger
		if logger == nil {
			logger = slog.Default()
		}
		logger.Warn("OpAMP session limit reached; rejecting session", "limit", maxActiveSupervisorSessions)
		if err := connection.Disconnect(); err != nil {
			logger.Warn("OpAMP session limit reached; disconnect failed", "error", err)
		}
		return
	}
	s.sessionCancels[connection] = cancel
	s.mu.Unlock()
	go s.monitorSession(ctx, connection, client, defaultSessionRevalidationInterval)
}

func (s *Server) stopSessionMonitor(connection opamptypes.Connection) {
	s.mu.Lock()
	cancel := s.sessionCancels[connection]
	delete(s.sessionCancels, connection)
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *Server) monitorSession(ctx context.Context, connection opamptypes.Connection, client coreapi.AgentManagementAPI, interval time.Duration) {
	if ctx == nil || connection == nil || client == nil || interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			requestContext, cancel := context.WithTimeout(ctx, 5*time.Second)
			_, err := client.GetSession(requestContext)
			cancel()
			var apiError *coreapi.ControlAPIError
			if errors.As(err, &apiError) && (apiError.Status == http.StatusUnauthorized || apiError.Status == http.StatusForbidden) {
				s.options.Logger.Warn("OpAMP installation authorization was revoked; disconnecting session")
				_ = connection.Disconnect()
				return
			}
			if err != nil {
				// Preserve the existing local Collector session during a Core outage.
				// Message handling still fails closed before delivering new policy.
				s.options.Logger.Debug("OpAMP session revalidation unavailable", "error", err)
			}
		}
	}
}

func (s *Server) handleMessage(ctx context.Context, client coreapi.AgentManagementAPI, message *protobufs.AgentToServer, sessionConfigs *sessionConfigCache) *protobufs.ServerToAgent {
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
	body, hash, err := RenderGatewayConfig(session.DesiredPolicy)
	if err != nil {
		s.reportRemoteStatus(ctx, client, uid, message.GetRemoteConfigStatus(), session.DesiredPolicy.Revision)
		s.options.Logger.Warn("Core Gateway policy is invalid", "error", err)
		return &protobufs.ServerToAgent{InstanceUid: uidBytes}
	}
	hashText := hex.EncodeToString(hash[:])
	statusConfirmsDesired := message.GetRemoteConfigStatus() != nil &&
		bytes.Equal(message.GetRemoteConfigStatus().GetLastRemoteConfigHash(), hash[:]) &&
		(message.GetRemoteConfigStatus().GetStatus() == protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLYING ||
			message.GetRemoteConfigStatus().GetStatus() == protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED)
	s.reportRemoteStatus(ctx, client, uid, message.GetRemoteConfigStatus(), session.DesiredPolicy.Revision)
	publicationCtx, cancelPublication := context.WithTimeout(ctx, 5*time.Second)
	publication, err := client.RecordPublication(publicationCtx, coreapi.PublicationRequest{
		InstanceUID: uid, PolicySchemaVersion: session.DesiredPolicy.SchemaVersion,
		PolicyRevision: session.DesiredPolicy.Revision, ConfigHash: hashText,
	})
	cancelPublication()
	if err != nil || publication.ConfigHash != hashText || publication.PolicyRevision != session.DesiredPolicy.Revision || publication.InstanceUID != uid {
		s.options.Logger.Warn("Core configuration publication could not be persisted", "error", err)
		return &protobufs.ServerToAgent{InstanceUid: uidBytes}
	}
	// The OpAMP hash identifies config content, not the Core policy revision.
	// Refresh this association before interpreting status so an unchanged
	// endpoint at a newer revision is credited to the current desired policy.
	s.markSent(uid, hashText, session.DesiredPolicy.Revision)
	if sessionConfigs == nil {
		sessionConfigs = &sessionConfigCache{}
	}
	if statusConfirmsDesired {
		sessionConfigs.markSent(uid, hashText)
		return response
	}
	statusFailed := message.GetRemoteConfigStatus() != nil &&
		message.GetRemoteConfigStatus().GetStatus() == protobufs.RemoteConfigStatuses_RemoteConfigStatuses_FAILED
	if sessionConfigs.wasSent(uid, hashText) && !statusFailed {
		return response
	}
	response.RemoteConfig = &protobufs.AgentRemoteConfig{
		Config:     &protobufs.AgentConfigMap{ConfigMap: map[string]*protobufs.AgentConfigFile{"": {Body: body}}},
		ConfigHash: hash[:],
	}
	sessionConfigs.markSent(uid, hashText)
	return response
}

func (s *Server) reportRemoteStatus(ctx context.Context, client coreapi.AgentManagementAPI, uid string, status *protobufs.RemoteConfigStatus, desiredRevision int64) {
	if status == nil || len(status.GetLastRemoteConfigHash()) != 32 {
		return
	}
	configHash := hex.EncodeToString(status.GetLastRemoteConfigHash())
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	publication, err := client.ResolvePublication(callCtx, uid, configHash)
	cancel()
	if err != nil || publication.PolicyRevision < 1 || publication.PolicyRevision > desiredRevision {
		s.options.Logger.Debug("OpAMP remote config status has no durable publication", "instance_uid", uid)
		return
	}
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
	report := coreapi.SupervisorStatusReport{InstanceUID: uid, DesiredRevision: desiredRevision, PublicationRevision: publication.PolicyRevision, Status: statusName, ConfigHash: configHash}
	if statusName == "APPLIED" {
		report.AppliedRevision = publication.PolicyRevision
	}
	callCtx, cancel = context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.ReportStatus(callCtx, report); err != nil {
		s.options.Logger.Warn("OpAMP status report to Core failed", "error", err)
	}
}

func (s *Server) markSent(uid, hash string, revision int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sent == nil {
		s.sent = make(map[string]map[string]int64)
	}
	if s.lastUsed == nil {
		s.lastUsed = make(map[string]uint64)
	}
	if _, exists := s.sent[uid]; !exists && len(s.sent) >= maxTrackedInstances {
		s.evictLeastRecentlyUsedLocked()
	}
	if s.sent[uid] == nil {
		s.sent[uid] = make(map[string]int64)
	}
	if _, exists := s.sent[uid][hash]; !exists && len(s.sent[uid]) >= maxConfigHashesPerTrackedInstance {
		s.evictOldestRevisionLocked(uid)
	}
	s.sent[uid][hash] = revision
	s.touchLocked(uid)
}

func (s *Server) revisionForHash(uid, hash string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	revision := s.sent[uid][hash]
	if revision > 0 {
		s.touchLocked(uid)
	}
	return revision
}

func (s *Server) touchLocked(uid string) {
	s.sequence++
	s.lastUsed[uid] = s.sequence
}

func (s *Server) evictLeastRecentlyUsedLocked() {
	oldestUID := ""
	var oldestSequence uint64
	for uid, sequence := range s.lastUsed {
		if oldestUID == "" || sequence < oldestSequence {
			oldestUID, oldestSequence = uid, sequence
		}
	}
	if oldestUID != "" {
		delete(s.sent, oldestUID)
		delete(s.lastUsed, oldestUID)
	}
}

func (s *Server) evictOldestRevisionLocked(uid string) {
	oldestHash := ""
	var oldestRevision int64
	for hash, revision := range s.sent[uid] {
		if oldestHash == "" || revision < oldestRevision || revision == oldestRevision && hash < oldestHash {
			oldestHash, oldestRevision = hash, revision
		}
	}
	if oldestHash != "" {
		delete(s.sent[uid], oldestHash)
	}
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
