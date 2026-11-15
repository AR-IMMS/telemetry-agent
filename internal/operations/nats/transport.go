package nats

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/operations"
	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	maxCommandBytes = 64 * 1024
	maxOutboxBatch  = 16
)

// Config selects one externally provisioned installation-scoped durable consumer.
type Config struct {
	URL            string
	Username       string
	Password       string `json:"-"`
	CAFile         string
	InstallationID string
	BindingID      string
	Stream         string
	Durable        string
}

// BindingSchemaV1 identifies the versioned Core-provisioned Operations broker binding.
const BindingSchemaV1 = "ar-imms.operations.broker-binding.v1"

// BindingState is one Core-authoritative broker binding revision delivered through
// the protected local provisioning boundary. Password remains in protected storage.
type BindingState struct {
	SchemaVersion string `json:"schema_version"`
	Revision      string `json:"revision"`
	Active        bool   `json:"active"`
	Config        Config `json:"-"`
}

// BindingLoader reads the current protected binding state. An inactive state revokes
// only the Operations broker session; it does not affect telemetry or Collector work.
type BindingLoader func(context.Context) (BindingState, error)

// Client owns one authenticated, TLS-protected NATS connection and JetStream context.
type Client struct {
	connection *natsgo.Conn
	jetstream  jetstream.JetStream
	config     Config
}

// Runner reconnects Operations independently so an unavailable broker cannot stop telemetry.
type Runner struct {
	Config       Config
	Binding      BindingLoader
	Executor     *operations.Executor
	Outbox       ResultOutbox
	Retry        time.Duration
	PollInterval time.Duration
	connect      func(Config) (runnerSession, error)
}

type runnerSession interface {
	Run(context.Context, *operations.Executor, ResultOutbox) error
	Invalidate()
}

// Run keeps trying to establish the optional Operations connection until Agent shutdown.
func (r Runner) Run(ctx context.Context) error {
	if ctx == nil || r.Executor == nil || r.Outbox == nil {
		return fmt.Errorf("Operations runner, executor, outbox, and context are required")
	}
	retry := r.Retry
	if retry <= 0 || retry > time.Minute {
		retry = 5 * time.Second
	}
	for ctx.Err() == nil {
		config := r.Config
		var revision string
		if r.Binding != nil {
			binding, err := r.Binding(ctx)
			if err != nil || validateBindingState(binding) != nil || !binding.Active {
				if !wait(ctx, retry) {
					return nil
				}
				continue
			}
			config, revision = binding.Config, binding.Revision
		}
		dial := r.connect
		if dial == nil {
			dial = func(config Config) (runnerSession, error) { return Connect(config) }
		}
		client, err := dial(config)
		if err == nil {
			bindingUpdated := false
			runContext, cancel := context.WithCancel(ctx)
			finished := make(chan error, 1)
			go func() { finished <- client.Run(runContext, r.Executor, r.Outbox) }()
			if r.Binding == nil {
				select {
				case err = <-finished:
				case <-ctx.Done():
				}
			} else {
				poll := r.PollInterval
				if poll <= 0 || poll > time.Minute {
					poll = time.Second
				}
				ticker := time.NewTicker(poll)
				for {
					select {
					case err = <-finished:
						goto sessionDone
					case <-ctx.Done():
						goto sessionDone
					case <-ticker.C:
						current, loadErr := r.Binding(ctx)
						if loadErr != nil || validateBindingState(current) != nil || !current.Active {
							client.Invalidate()
							cancel()
							<-finished
							goto sessionDone
						}
						if current.Revision != revision || !reflect.DeepEqual(current.Config, config) {
							bindingUpdated = true
							client.Invalidate()
							cancel()
							<-finished
							goto sessionDone
						}
					}
				}
			sessionDone:
				ticker.Stop()
			}
			client.Invalidate()
			cancel()
			if ctx.Err() != nil {
				return nil
			}
			if bindingUpdated {
				continue
			}
		}
		if !wait(ctx, retry) {
			return nil
		}
	}
	return nil
}

func validateBindingState(binding BindingState) error {
	if binding.SchemaVersion != BindingSchemaV1 || strings.TrimSpace(binding.Revision) == "" {
		return fmt.Errorf("unsupported or incomplete Operations broker binding state")
	}
	if binding.Active {
		config := binding.Config
		if strings.TrimSpace(config.URL) == "" || config.Username == "" || config.Password == "" || config.CAFile == "" ||
			!safeToken(config.InstallationID) || !safeToken(config.BindingID) || !safeToken(config.Stream) || !safeToken(config.Durable) {
			return fmt.Errorf("active Operations broker binding is incomplete")
		}
	}
	return nil
}

func wait(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// Connect validates scoped credentials and establishes a TLS-only reconnecting connection.
func Connect(config Config) (*Client, error) {
	if strings.TrimSpace(config.URL) == "" || config.Username == "" || config.Password == "" ||
		config.CAFile == "" || !safeToken(config.InstallationID) || !safeToken(config.BindingID) ||
		!safeToken(config.Stream) || !safeToken(config.Durable) {
		return nil, fmt.Errorf("TLS endpoint, scoped credentials, installation, binding, stream, and durable consumer are required")
	}
	rootPEM, err := os.ReadFile(config.CAFile)
	if err != nil {
		return nil, fmt.Errorf("read Operations broker CA: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(rootPEM) {
		return nil, fmt.Errorf("Operations broker CA file contains no certificate")
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}
	connection, err := natsgo.Connect(config.URL,
		natsgo.UserInfo(config.Username, config.Password),
		natsgo.Secure(tlsConfig),
		natsgo.Name("ar-imms-agent-operations-"+config.InstallationID),
		natsgo.MaxReconnects(-1), natsgo.ReconnectWait(2*time.Second), natsgo.Timeout(5*time.Second))
	if err != nil {
		return nil, fmt.Errorf("connect to Operations broker")
	}
	js, err := jetstream.New(connection)
	if err != nil {
		connection.Close()
		return nil, fmt.Errorf("initialize JetStream client")
	}
	return &Client{connection: connection, jetstream: js, config: config}, nil
}

// Close closes the underlying NATS connection.
func (c *Client) Close() {
	if c != nil && c.connection != nil {
		c.connection.Drain()
	}
}

// Invalidate closes a provisioned broker session immediately during credential
// replacement or revocation. It deliberately does not drain stale subscriptions.
func (c *Client) Invalidate() {
	if c != nil && c.connection != nil {
		c.connection.Close()
	}
}

// ResultOutbox is the persistence port used by the publisher loop.
type ResultOutbox interface {
	Pending(context.Context, int) ([]operations.OutboxEvent, error)
	MarkPublished(context.Context, string, time.Time) error
	MarkFailed(context.Context, string, string, time.Time) error
}

// Run consumes one installation's durable commands and publishes committed results until cancellation.
func (c *Client) Run(ctx context.Context, executor *operations.Executor, outbox ResultOutbox) error {
	if ctx == nil || c == nil || c.jetstream == nil || executor == nil || outbox == nil {
		return fmt.Errorf("Operations transport, executor, journal, and context are required")
	}
	consumer, err := c.jetstream.Consumer(ctx, c.config.Stream, c.config.Durable)
	if err != nil {
		return fmt.Errorf("load provisioned durable Operations consumer")
	}
	consume, err := consumer.Consume(func(message jetstream.Msg) {
		c.handle(ctx, message, executor)
	}, jetstream.PullMaxMessages(1))
	if err != nil {
		return fmt.Errorf("start durable Operations consumer")
	}
	publisherDone := make(chan struct{})
	go func() {
		defer close(publisherDone)
		c.publishLoop(ctx, outbox)
	}()
	<-ctx.Done()
	consume.Drain()
	select {
	case <-consume.Closed():
	case <-time.After(5 * time.Second):
		consume.Stop()
	}
	<-publisherDone
	return nil
}

func (c *Client) handle(ctx context.Context, message jetstream.Msg, executor *operations.Executor) {
	if ctx.Err() != nil {
		_ = message.NakWithDelay(time.Second)
		return
	}
	if message.Subject() != "arimms.ops.v1.request."+c.config.InstallationID || len(message.Data()) == 0 || len(message.Data()) > maxCommandBytes {
		_ = message.TermWithReason("invalid_operation_subject_or_size")
		return
	}
	decoder := json.NewDecoder(strings.NewReader(string(message.Data())))
	decoder.DisallowUnknownFields()
	var request operations.Request
	if err := decoder.Decode(&request); err != nil {
		_ = message.TermWithReason("invalid_operation_envelope")
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		_ = message.TermWithReason("invalid_operation_envelope")
		return
	}
	progressDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-progressDone:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = message.InProgress()
			}
		}
	}()
	_, err := executor.Execute(ctx, request)
	close(progressDone)
	if err != nil {
		if errors.Is(err, operations.ErrInvalidEnvelope) || errors.Is(err, operations.ErrOperationConflict) || errors.Is(err, operations.ErrUnsupportedType) {
			_ = message.TermWithReason("operation_rejected")
			return
		}
		_ = message.NakWithDelay(time.Second)
		return
	}
	_ = message.Ack()
}

func (c *Client) publishLoop(ctx context.Context, outbox ResultOutbox) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		c.publishPending(ctx, outbox)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (c *Client) publishPending(ctx context.Context, outbox ResultOutbox) {
	events, err := outbox.Pending(ctx, maxOutboxBatch)
	if err != nil {
		return
	}
	for _, event := range events {
		if ctx.Err() != nil {
			return
		}
		message := &natsgo.Msg{Subject: event.Subject, Data: event.Payload}
		if _, err := c.jetstream.PublishMsg(ctx, message, jetstream.WithMsgID(event.EventID)); err != nil {
			delay := retryDelay(event.Attempts)
			_ = outbox.MarkFailed(ctx, event.EventID, "BROKER_PUBLISH_FAILED", time.Now().UTC().Add(delay))
			continue
		}
		_ = outbox.MarkPublished(ctx, event.EventID, time.Now().UTC())
	}
}

func retryDelay(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	if attempt > 8 {
		attempt = 8
	}
	delay := time.Second * time.Duration(1<<attempt)
	if delay > 5*time.Minute {
		return 5 * time.Minute
	}
	return delay
}

func safeToken(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}
