package librehardwaremonitor

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// WaitForHealth polls the local LHM metrics endpoint until it returns HTTP 200
// or the supplied context is cancelled.
func WaitForHealth(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	pollInterval time.Duration,
) error {
	if client == nil {
		return fmt.Errorf("Libre Hardware Monitor HTTP client is required")
	}
	if strings.TrimSpace(endpoint) == "" {
		return fmt.Errorf(
			"Libre Hardware Monitor health endpoint is required",
		)
	}
	if pollInterval <= 0 {
		return fmt.Errorf(
			"Libre Hardware Monitor health poll interval must be positive",
		)
	}

	for {
		request, err := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			endpoint,
			nil,
		)
		if err != nil {
			return fmt.Errorf(
				"create Libre Hardware Monitor health request: %w",
				err,
			)
		}

		response, requestErr := client.Do(request)

		if requestErr == nil {
			response.Body.Close()

			if response.StatusCode == http.StatusOK {
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
