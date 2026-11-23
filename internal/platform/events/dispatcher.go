package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
)

var (
	// ErrQueueFull reports that a nonblocking advisory publish was not admitted.
	ErrQueueFull = errors.New("event queue is full")
	// ErrNotStarted reports that publish was attempted before Start.
	ErrNotStarted = errors.New("event dispatcher has not started")
	// ErrStopped reports that the dispatcher has stopped or is stopping.
	ErrStopped = errors.New("event dispatcher is stopped")
	// ErrInvalidEvent reports a desired-state event without a valid revision.
	ErrInvalidEvent = errors.New("desired configuration revision must be positive")
)

// DesiredConfigurationChanged signals that a desired-state revision committed.
// The revision is a hint; subscribers re-read the durable repository snapshot.
type DesiredConfigurationChanged struct {
	Revision uint64
}

// Handler processes one desired-state wakeup. It must honor ctx for shutdown.
type Handler func(context.Context, DesiredConfigurationChanged) error

// Publisher admits a typed advisory event without blocking on queue capacity.
type Publisher interface {
	Publish(DesiredConfigurationChanged) error
}

// Dispatcher delivers desired-state wakeups in FIFO order to one serial handler.
type Dispatcher struct {
	queue   chan DesiredConfigurationChanged
	handler Handler

	mu      sync.Mutex
	started bool
	stopped bool
	runCtx  context.Context
	cancel  context.CancelFunc
	done    chan struct{}
}

// NewDispatcher creates a dispatcher with a fixed bounded queue and handler.
func NewDispatcher(capacity int, handler Handler) (*Dispatcher, error) {
	if capacity <= 0 || handler == nil {
		return nil, fmt.Errorf("positive event capacity and handler are required")
	}
	return &Dispatcher{queue: make(chan DesiredConfigurationChanged, capacity), handler: handler, done: make(chan struct{})}, nil
}

// Start starts the dispatcher once and binds it to ctx cancellation.
func (d *Dispatcher) Start(ctx context.Context) error {
	if d == nil || ctx == nil {
		return fmt.Errorf("event dispatcher and context are required")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.started || d.stopped {
		return fmt.Errorf("event dispatcher can only be started once")
	}
	runContext, cancel := context.WithCancel(ctx)
	d.started = true
	d.runCtx = runContext
	d.cancel = cancel
	go d.run(runContext)
	return nil
}

// Publish queues an event without blocking. Queue saturation is reported to
// the caller and does not affect the already-committed state mutation.
func (d *Dispatcher) Publish(event DesiredConfigurationChanged) error {
	if d == nil {
		return ErrStopped
	}
	if event.Revision == 0 {
		return ErrInvalidEvent
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.started {
		return ErrNotStarted
	}
	if d.stopped || d.runCtx.Err() != nil {
		return ErrStopped
	}
	select {
	case d.queue <- event:
		return nil
	default:
		return ErrQueueFull
	}
}

// Shutdown cancels delivery and waits until the active handler returns or ctx
// expires. Queued events may be discarded because SQLite and polling recover.
func (d *Dispatcher) Shutdown(ctx context.Context) error {
	if d == nil || ctx == nil {
		return fmt.Errorf("event dispatcher and shutdown context are required")
	}
	d.mu.Lock()
	if !d.started {
		if d.stopped {
			done := d.done
			d.mu.Unlock()
			select {
			case <-done:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		d.stopped = true
		close(d.done)
		d.mu.Unlock()
		return nil
	}
	d.stopped = true
	if d.cancel != nil {
		d.cancel()
	}
	done := d.done
	d.mu.Unlock()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (d *Dispatcher) run(ctx context.Context) {
	defer func() {
		d.mu.Lock()
		d.stopped = true
		close(d.done)
		d.mu.Unlock()
	}()
	for {
		if ctx.Err() != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case event := <-d.queue:
			if ctx.Err() != nil {
				return
			}
			if err := invoke(ctx, d.handler, event); err != nil {
				if ctx.Err() == nil {
					slog.Error("Agent advisory event handler failed", "code", "event_handler_failed")
				}
			}
		}
	}
}

func invoke(ctx context.Context, handler Handler, event DesiredConfigurationChanged) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("event handler panic")
		}
	}()
	return handler(ctx, event)
}
