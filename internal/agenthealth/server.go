package agenthealth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
)

// ServeStatus serves the local Agent health endpoint until ctx is cancelled.
func ServeStatus(
	ctx context.Context,
	listener net.Listener,
	provider Provider,
) error {
	if ctx == nil {
		return fmt.Errorf("status server context is required")
	}
	if listener == nil {
		return fmt.Errorf("status server listener is required")
	}

	server := &http.Server{
		Handler: NewHTTPHandler(provider),
	}

	stopped := make(chan struct{})

	go func() {
		select {
		case <-ctx.Done():
			_ = server.Close()

		case <-stopped:
		}
	}()

	err := server.Serve(listener)
	close(stopped)

	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("serve Agent status endpoint: %w", err)
	}

	return nil
}
