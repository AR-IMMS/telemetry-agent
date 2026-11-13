package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/open-telemetry/opamp-go/server"
	opamptypes "github.com/open-telemetry/opamp-go/server/types"
	"google.golang.org/protobuf/proto"
)

func stopPR07ASupervisorProcess(process *os.Process) {
	if process == nil {
		return
	}
	if runtime.GOOS == "windows" {
		_ = exec.Command("taskkill.exe", "/PID", fmt.Sprint(process.Pid), "/T", "/F").Run()
		return
	}
	_ = process.Signal(os.Interrupt)
}

type pr07aOpAMPHarness struct {
	server interface {
		Start(server.StartSettings) error
		Stop(context.Context) error
	}
	mu           sync.RWMutex
	config       []byte
	lastSentHash []byte
	status       chan *protobufs.RemoteConfigStatus
	connected    chan struct{}
}

func startPR07AOpAMPServer(t *testing.T, endpoint string, initialConfig []byte) *pr07aOpAMPHarness {
	t.Helper()
	harness := &pr07aOpAMPHarness{config: append([]byte(nil), initialConfig...), status: make(chan *protobufs.RemoteConfigStatus, 32), connected: make(chan struct{})}
	harness.server = server.New(nil)
	callbacks := opamptypes.Callbacks{OnConnecting: func(*http.Request) opamptypes.ConnectionResponse {
		return opamptypes.ConnectionResponse{Accept: true, ConnectionCallbacks: opamptypes.ConnectionCallbacks{
			OnConnected: func(context.Context, opamptypes.Connection) {
				select {
				case <-harness.connected:
				default:
					close(harness.connected)
				}
			},
			OnMessage: func(_ context.Context, _ opamptypes.Connection, message *protobufs.AgentToServer) *protobufs.ServerToAgent {
				if message.GetRemoteConfigStatus() != nil {
					select {
					case harness.status <- proto.Clone(message.GetRemoteConfigStatus()).(*protobufs.RemoteConfigStatus):
					default:
					}
				}
				harness.mu.Lock()
				defer harness.mu.Unlock()
				config := append([]byte(nil), harness.config...)
				response := &protobufs.ServerToAgent{
					InstanceUid:  message.GetInstanceUid(),
					Capabilities: uint64(protobufs.ServerCapabilities_ServerCapabilities_OffersRemoteConfig),
				}
				if config != nil {
					hash := sha256.Sum256(config)
					if !bytes.Equal(harness.lastSentHash, hash[:]) {
						response.RemoteConfig = &protobufs.AgentRemoteConfig{
							Config:     &protobufs.AgentConfigMap{ConfigMap: map[string]*protobufs.AgentConfigFile{"": {Body: config}}},
							ConfigHash: hash[:],
						}
						harness.lastSentHash = append(harness.lastSentHash[:0], hash[:]...)
					}
				}
				return response
			},
		}}
	}}
	if err := harness.server.Start(server.StartSettings{
		Settings: server.Settings{Callbacks: callbacks}, ListenEndpoint: endpoint, ListenPath: "/v1/opamp",
	}); err != nil {
		t.Fatalf("start opamp-go v0.23.0 test server: %v", err)
	}
	return harness
}

func (h *pr07aOpAMPHarness) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = h.server.Stop(ctx)
}

func waitPR07ARemoteConfigApplied(t *testing.T, ctx context.Context, done <-chan error, harness *pr07aOpAMPHarness, label string) {
	t.Helper()
	want := sha256.Sum256(harness.currentConfig())
	timer := time.NewTimer(90 * time.Second)
	defer timer.Stop()
	for {
		select {
		case err := <-done:
			t.Fatalf("Supervisor exited while waiting for %s: %v", label, err)
		case status := <-harness.status:
			if status.GetStatus() == protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED && string(status.GetLastRemoteConfigHash()) == string(want[:]) {
				return
			}
		case <-ctx.Done():
			t.Fatalf("context ended waiting for %s", label)
		case <-timer.C:
			t.Fatalf("timed out waiting for %s; expected config hash %x", label, want)
		}
	}
}

func (h *pr07aOpAMPHarness) currentConfig() []byte {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return append([]byte(nil), h.config...)
}

func (h *pr07aOpAMPHarness) setRemoteConfig(config []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.config = append([]byte(nil), config...)
}

func drainPR07ARemoteStatuses(harness *pr07aOpAMPHarness) {
	for {
		select {
		case <-harness.status:
		default:
			return
		}
	}
}

func observePR07ARemoteStatus(ctx context.Context, done <-chan error, harness *pr07aOpAMPHarness, wantStatus protobufs.RemoteConfigStatuses, wantHash []byte, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	for {
		select {
		case <-done:
			return false
		case status := <-harness.status:
			if status.GetStatus() == wantStatus && string(status.GetLastRemoteConfigHash()) == string(wantHash) {
				return true
			}
		case <-ctx.Done():
			return false
		case <-timer.C:
			return false
		}
	}
}

func waitPR07ARemoteStatus(t *testing.T, ctx context.Context, done <-chan error, harness *pr07aOpAMPHarness, wantStatus protobufs.RemoteConfigStatuses, wantHash []byte, label string) {
	t.Helper()
	timer := time.NewTimer(90 * time.Second)
	defer timer.Stop()
	for {
		select {
		case err := <-done:
			t.Fatalf("Supervisor exited while waiting for %s: %v", label, err)
		case status := <-harness.status:
			if status.GetStatus() == wantStatus && string(status.GetLastRemoteConfigHash()) == string(wantHash) {
				return
			}
		case <-ctx.Done():
			t.Fatalf("context ended waiting for %s", label)
		case <-timer.C:
			t.Fatalf("timed out waiting for %s status=%s hash=%x", label, wantStatus, wantHash)
		}
	}
}

func killPR07AManagedCollector(t *testing.T, supervisorOutput string) {
	t.Helper()
	const marker = `"msg":"Agent process started","pid":`
	index := strings.LastIndex(supervisorOutput, marker)
	if index < 0 {
		t.Fatalf("could not find managed Collector PID in Supervisor output: %s", tailPR07ATestOutput(supervisorOutput, 50))
	}
	start := index + len(marker)
	end := start
	for end < len(supervisorOutput) && supervisorOutput[end] >= '0' && supervisorOutput[end] <= '9' {
		end++
	}
	pid, err := strconv.Atoi(supervisorOutput[start:end])
	if err != nil || pid <= 0 {
		t.Fatalf("parse managed Collector PID from %q: %v", supervisorOutput[start:end], err)
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		t.Fatalf("find managed Collector process %d: %v", pid, err)
	}
	if err := process.Kill(); err != nil {
		t.Fatalf("kill managed Collector process %d for recovery test: %v", pid, err)
	}
}

func waitPR07ACollectorRestart(t *testing.T, ctx context.Context, done <-chan error, output *lockedTemplateE2EBuffer, previousStarts int) {
	t.Helper()
	deadline := time.NewTimer(60 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if strings.Count(output.String(), "Agent process started") > previousStarts {
			return
		}
		select {
		case err := <-done:
			t.Fatalf("Supervisor exited during Collector crash recovery: %v", err)
		case <-ctx.Done():
			t.Fatalf("context ended waiting for Collector restart")
		case <-deadline.C:
			t.Fatalf("timed out waiting for Collector restart after induced crash: %s", tailPR07ATestOutput(output.String(), 50))
		case <-ticker.C:
		}
	}
}

func waitPR07ASinkGrowth(t *testing.T, ctx context.Context, done <-chan error, sink *lockedTemplateE2EBuffer, previousSize int, label string) {
	t.Helper()
	deadline := time.NewTimer(60 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if len(sink.String()) > previousSize {
			return
		}
		select {
		case err := <-done:
			t.Fatalf("Supervisor exited waiting for %s: %v", label, err)
		case <-ctx.Done():
			t.Fatalf("context ended waiting for %s", label)
		case <-deadline.C:
			t.Fatalf("timed out waiting for %s", label)
		case <-ticker.C:
		}
	}
}

func writePR07ACollectorConfig(t *testing.T, path, endpoint string) {
	t.Helper()
	content := fmt.Sprintf(`receivers:
  hostmetrics:
    collection_interval: 1s
    scrapers:
      cpu: {}
      memory: {}
exporters:
  otlp:
    endpoint: %q
    tls:
      insecure: true
service:
  pipelines:
    metrics:
      receivers: [hostmetrics]
      exporters: [otlp]
`, endpoint)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func reservePR07ATCPAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}

func waitPR07ASinkContains(t *testing.T, ctx context.Context, done <-chan error, sink *lockedTemplateE2EBuffer, marker, label string) {
	t.Helper()
	timer := time.NewTimer(90 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if strings.Contains(sink.String(), marker) {
			return
		}
		select {
		case err := <-done:
			t.Fatalf("Supervisor exited waiting for %s: %v output=%s", label, err, sink.String())
		case <-ctx.Done():
			t.Fatalf("context ended waiting for %s", label)
		case <-timer.C:
			t.Fatalf("timed out waiting for %s: %s", label, tailPR07ATestOutput(sink.String(), 30))
		case <-ticker.C:
		}
	}
}

func tailPR07ATestOutput(value string, lines int) string {
	parts := strings.Split(strings.TrimSpace(value), "\n")
	if len(parts) > lines {
		parts = parts[len(parts)-lines:]
	}
	return strings.Join(parts, "\n")
}

func writePR07AEvidence(t *testing.T, directory string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatalf("create PR-07a evidence directory: %v", err)
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(contents+"\n"), 0600); err != nil {
			t.Fatalf("write PR-07a evidence %s: %v", name, err)
		}
	}
}
