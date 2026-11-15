package mockcore

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/ar-imms/telemetry-agent/internal/operations"
	"github.com/nats-io/nats.go/jetstream"
)

const maxResultBytes = 64 * 1024

// Core publishes operations and records each result event once for E2E assertions.
type Core struct {
	jetstream     jetstream.JetStream
	installation  string
	requestStream string
	resultStream  string
	consumerName  string
	mu            sync.Mutex
	seen          map[string]struct{}
	bindings      map[string]BrokerBinding
	results       chan operations.Result
	consumer      jetstream.ConsumeContext
}

// New creates a Core test adapter over a separately authenticated broker connection.
func New(js jetstream.JetStream, installationID, requestStream, resultStream, resultConsumer string) (*Core, error) {
	if js == nil || installationID == "" || requestStream == "" || resultStream == "" || resultConsumer == "" {
		return nil, fmt.Errorf("Mock Core JetStream, installation, streams, and result consumer are required")
	}
	return &Core{jetstream: js, installation: installationID, requestStream: requestStream, resultStream: resultStream,
		consumerName: resultConsumer, seen: make(map[string]struct{}), results: make(chan operations.Result, 32)}, nil
}

// Start begins consuming the already provisioned result durable.
func (c *Core) Start(ctx context.Context) error {
	consumer, err := c.jetstream.Consumer(ctx, c.resultStream, c.consumerName)
	if err != nil {
		return fmt.Errorf("load Mock Core result durable")
	}
	consume, err := consumer.Consume(c.accept, jetstream.PullMaxMessages(1))
	if err != nil {
		return fmt.Errorf("start Mock Core result consumer")
	}
	c.consumer = consume
	return nil
}

// PublishOperation sends one test-authorized request using operation ID deduplication.
func (c *Core) PublishOperation(ctx context.Context, request operations.Request) error {
	return c.PublishOperationWithMessageID(ctx, request, request.OperationID)
}

// PublishOperationWithMessageID redelivers a logical operation with a distinct JetStream message ID.
func (c *Core) PublishOperationWithMessageID(ctx context.Context, request operations.Request, messageID string) error {
	if request.InstallationID != c.installation || request.OperationID == "" {
		return fmt.Errorf("Mock Core request installation and operation identity are required")
	}
	if messageID == "" || len(messageID) > 128 {
		return fmt.Errorf("Mock Core transport message identity is required")
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode Mock Core operation request")
	}
	_, err = c.jetstream.Publish(ctx, "arimms.ops.v1.request."+c.installation, payload, jetstream.WithMsgID(messageID))
	if err != nil {
		return fmt.Errorf("publish Mock Core operation request")
	}
	return nil
}

// Results exposes unique, acknowledged Agent results to the integration test.
func (c *Core) Results() <-chan operations.Result { return c.results }

// Close drains the Core result consumer.
func (c *Core) Close() {
	if c != nil && c.consumer != nil {
		c.consumer.Drain()
	}
}

func (c *Core) accept(message jetstream.Msg) {
	if len(message.Data()) == 0 || len(message.Data()) > maxResultBytes || message.Subject() != "arimms.ops.v1.result."+c.installation {
		_ = message.TermWithReason("invalid_mockcore_result")
		return
	}
	var result operations.Result
	if err := json.Unmarshal(message.Data(), &result); err != nil || result.InstallationID != c.installation || result.EventID == "" {
		_ = message.TermWithReason("invalid_mockcore_result")
		return
	}
	c.mu.Lock()
	if _, exists := c.seen[result.EventID]; exists {
		c.mu.Unlock()
		_ = message.Ack()
		return
	}
	c.seen[result.EventID] = struct{}{}
	c.mu.Unlock()
	select {
	case c.results <- result:
		_ = message.Ack()
	default:
		c.mu.Lock()
		delete(c.seen, result.EventID)
		c.mu.Unlock()
		_ = message.NakWithDelay(1)
	}
}
