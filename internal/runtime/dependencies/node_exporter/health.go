package nodeexporter

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// WaitForHealth retries until the Node Exporter endpoint returns a 2xx response
// or the supplied context is cancelled.
func WaitForHealth(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	pollInterval time.Duration,
) error {
	if client == nil {
		return fmt.Errorf("Node Exporter health HTTP client is required")
	}
	if strings.TrimSpace(endpoint) == "" {
		return fmt.Errorf("Node Exporter health endpoint is required")
	}
	if pollInterval <= 0 {
		return fmt.Errorf("Node Exporter health poll interval must be positive")
	}

	for {
		request, err := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			endpoint,
			nil,
		)
		if err != nil {
			return fmt.Errorf("create Node Exporter health request: %w", err)
		}

		response, requestErr := client.Do(request)

		if requestErr == nil {
			response.Body.Close()

			if response.StatusCode >= http.StatusOK &&
				response.StatusCode < http.StatusMultipleChoices {
				return nil
			}
		}

		timer := time.NewTimer(pollInterval)

		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}

			return ctx.Err()

		case <-timer.C:
		}
	}
}
