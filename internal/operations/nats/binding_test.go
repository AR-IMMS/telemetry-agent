package nats

import (
	"context"
	"sync"
	"testing"
	"time"

	operations "github.com/ar-imms/telemetry-agent/internal/operations/command"
)

func TestRunnerInvalidatesSessionWhenBindingCredentialIsRevoked(t *testing.T) {
	state := BindingState{SchemaVersion: BindingSchemaV1, Revision: "rev-1", Active: true,
		Config: Config{URL: "tls://broker.test:4222", Username: "agent", Password: "old-secret", CAFile: "ca.pem",
			InstallationID: "installation-1", BindingID: "binding-1", Stream: "REQUESTS", Durable: "AGENT_1"}}
	var stateMu sync.RWMutex
	load := func(context.Context) (BindingState, error) {
		stateMu.RLock()
		defer stateMu.RUnlock()
		return state, nil
	}
	opened := make(chan *fakeRunnerSession, 2)
	runner := Runner{Executor: &operations.Executor{}, Outbox: fakeRunnerOutbox{}, Binding: load,
		PollInterval: 10 * time.Millisecond, connect: func(config Config) (runnerSession, error) {
			session := &fakeRunnerSession{done: make(chan struct{}), closed: make(chan struct{}), config: config}
			opened <- session
			return session, nil
		}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx) }()

	var session *fakeRunnerSession
	select {
	case session = <-opened:
	case <-time.After(time.Second):
		t.Fatal("runner did not establish the provisioned binding")
	}
	stateMu.Lock()
	state = BindingState{SchemaVersion: BindingSchemaV1, Revision: "rev-2", Active: true,
		Config: Config{URL: "tls://broker.test:4222", Username: "agent", Password: "new-secret", CAFile: "ca.pem",
			InstallationID: "installation-1", BindingID: "binding-1", Stream: "REQUESTS", Durable: "AGENT_1"}}
	stateMu.Unlock()
	select {
	case <-session.closed:
	case <-time.After(time.Second):
		t.Fatal("credential replacement did not invalidate the active Operations session")
	}
	select {
	case session = <-opened:
		if session.config.Password != "new-secret" {
			t.Fatal("runner did not use the replacement binding credential")
		}
	case <-time.After(time.Second):
		t.Fatal("runner did not reconnect with the replacement credential")
	}
	stateMu.Lock()
	state = BindingState{SchemaVersion: BindingSchemaV1, Revision: "rev-3", Active: false}
	stateMu.Unlock()
	select {
	case <-session.closed:
	case <-time.After(time.Second):
		t.Fatal("revocation did not invalidate the active Operations session")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runner returned error after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("runner did not stop after cancellation")
	}
}

type fakeRunnerSession struct {
	done   chan struct{}
	closed chan struct{}
	one    sync.Once
	config Config
}

func (s *fakeRunnerSession) Run(ctx context.Context, _ *operations.Executor, _ ResultOutbox) error {
	select {
	case <-ctx.Done():
		return nil
	case <-s.done:
		return nil
	}
}

func (s *fakeRunnerSession) Close() {
	s.one.Do(func() {
		close(s.done)
		if s.closed != nil {
			close(s.closed)
		}
	})
}

func (s *fakeRunnerSession) Invalidate() { s.Close() }

type fakeRunnerOutbox struct{}

func (fakeRunnerOutbox) Pending(context.Context, int) ([]operations.OutboxEvent, error) {
	return nil, nil
}
func (fakeRunnerOutbox) MarkPublished(context.Context, string, time.Time) error      { return nil }
func (fakeRunnerOutbox) MarkFailed(context.Context, string, string, time.Time) error { return nil }
