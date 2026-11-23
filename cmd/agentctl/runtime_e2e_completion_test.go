package main

import (
	"errors"
	"testing"
	"time"
)

func startRuntimeE2E(run func() error) <-chan error {
	done := make(chan error, 1)
	go func() {
		defer close(done)
		done <- run()
	}()
	return done
}

func TestRuntimeE2ECompletionRemainsObservableAfterResultIsRead(t *testing.T) {
	want := errors.New("runtime startup failed")
	done := startRuntimeE2E(func() error { return want })

	select {
	case got := <-done:
		if !errors.Is(got, want) {
			t.Fatalf("runtime result = %v, want %v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("runtime result was not delivered")
	}

	select {
	case _, open := <-done:
		if open {
			t.Fatal("runtime completion channel still accepts values")
		}
	case <-time.After(time.Second):
		t.Fatal("runtime completion was not observable after reading its result")
	}
}
