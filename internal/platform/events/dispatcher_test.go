package events

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestDispatcherPreservesFIFOAndContinuesAfterHandlerError(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	seen := make(chan uint64, 2)
	dispatcher, err := NewDispatcher(2, func(_ context.Context, event DesiredConfigurationChanged) error {
		seen <- event.Revision
		if event.Revision == 1 {
			close(entered)
			<-release
			return errors.New("expected test error")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Publish(DesiredConfigurationChanged{Revision: 1}); err != nil {
		t.Fatal(err)
	}
	<-entered
	if err := dispatcher.Publish(DesiredConfigurationChanged{Revision: 2}); err != nil {
		t.Fatal(err)
	}
	close(release)
	for _, want := range []uint64{1, 2} {
		select {
		case got := <-seen:
			if got != want {
				t.Fatalf("handler revision = %d, want %d", got, want)
			}
		case <-time.After(time.Second):
			t.Fatal("dispatcher did not deliver queued event")
		}
	}
	shutdownDispatcher(t, dispatcher)
}

func TestDispatcherReportsBoundedQueueOverflow(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	dispatcher, err := NewDispatcher(1, func(context.Context, DesiredConfigurationChanged) error {
		select {
		case <-entered:
		default:
			close(entered)
		}
		<-release
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Publish(DesiredConfigurationChanged{Revision: 1}); err != nil {
		t.Fatal(err)
	}
	<-entered
	if err := dispatcher.Publish(DesiredConfigurationChanged{Revision: 2}); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Publish(DesiredConfigurationChanged{Revision: 3}); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("overflow Publish() error = %v, want ErrQueueFull", err)
	}
	close(release)
	shutdownDispatcher(t, dispatcher)
}

func TestDispatcherIsolatesPanicAndContinues(t *testing.T) {
	seen := make(chan uint64, 2)
	dispatcher, err := NewDispatcher(2, func(_ context.Context, event DesiredConfigurationChanged) error {
		seen <- event.Revision
		if event.Revision == 1 {
			panic("expected test panic")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, revision := range []uint64{1, 2} {
		if err := dispatcher.Publish(DesiredConfigurationChanged{Revision: revision}); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range []uint64{1, 2} {
		select {
		case got := <-seen:
			if got != want {
				t.Fatalf("handler revision = %d, want %d", got, want)
			}
		case <-time.After(time.Second):
			t.Fatal("dispatcher did not continue after panic")
		}
	}
	shutdownDispatcher(t, dispatcher)
}

func TestDispatcherShutdownHonorsDeadlineForUncooperativeHandler(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	dispatcher, err := NewDispatcher(1, func(context.Context, DesiredConfigurationChanged) error {
		close(entered)
		<-release
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Publish(DesiredConfigurationChanged{Revision: 1}); err != nil {
		t.Fatal(err)
	}
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := dispatcher.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown() error = %v, want deadline exceeded", err)
	}
	close(release)
	shutdownDispatcher(t, dispatcher)
}

func TestDispatcherStopsOnParentCancellationAndRejectsFurtherPublish(t *testing.T) {
	entered := make(chan struct{})
	finished := make(chan struct{})
	parent, cancelParent := context.WithCancel(context.Background())
	defer cancelParent()
	dispatcher, err := NewDispatcher(1, func(ctx context.Context, _ DesiredConfigurationChanged) error {
		close(entered)
		<-ctx.Done()
		close(finished)
		return ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Start(parent); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Publish(DesiredConfigurationChanged{Revision: 1}); err != nil {
		t.Fatal(err)
	}
	<-entered
	cancelParent()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("handler did not receive parent cancellation")
	}
	shutdownDispatcher(t, dispatcher)
	if err := dispatcher.Publish(DesiredConfigurationChanged{Revision: 2}); !errors.Is(err, ErrStopped) {
		t.Fatalf("Publish() after cancellation error = %v, want ErrStopped", err)
	}
}

func TestDispatcherShutdownBeforeStartIsIdempotent(t *testing.T) {
	dispatcher, err := NewDispatcher(1, func(context.Context, DesiredConfigurationChanged) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	shutdownDispatcher(t, dispatcher)
	shutdownDispatcher(t, dispatcher)
	if err := dispatcher.Start(context.Background()); err == nil {
		t.Fatal("Start() after shutdown unexpectedly succeeded")
	}
}

func TestDispatcherRejectsConcurrentPublishAfterStart(t *testing.T) {
	const count = 32
	var mu sync.Mutex
	seen := 0
	dispatcher, err := NewDispatcher(count, func(context.Context, DesiredConfigurationChanged) error {
		mu.Lock()
		seen++
		mu.Unlock()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	for i := 1; i <= count; i++ {
		wait.Add(1)
		go func(revision uint64) {
			defer wait.Done()
			if err := dispatcher.Publish(DesiredConfigurationChanged{Revision: revision}); err != nil {
				t.Errorf("Publish() error = %v", err)
			}
		}(uint64(i))
	}
	wait.Wait()
	deadline := time.Now().Add(time.Second)
	for {
		mu.Lock()
		got := seen
		mu.Unlock()
		if got == count {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("handled %d concurrent events, want %d", got, count)
		}
		time.Sleep(time.Millisecond)
	}
	shutdownDispatcher(t, dispatcher)
}

func shutdownDispatcher(t *testing.T, dispatcher *Dispatcher) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := dispatcher.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}
