package mockcore

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/coreapi"
	"github.com/ar-imms/telemetry-agent/internal/coreconfig"
)

type persistentSnapshot struct {
	Installations []persistentInstallation           `json:"installations"`
	Publications  []coreapi.ConfigurationPublication `json:"publications"`
	Statuses      []StatusObservation                `json:"statuses"`
}

type persistentInstallation struct {
	TokenHash string                  `json:"token_hash"`
	Value     ProvisionedInstallation `json:"value"`
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
