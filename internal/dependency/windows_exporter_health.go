package dependency

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// waitForWindowsExporterHealth polls the exporter health endpoint until it
// returns a successful HTTP status or the supplied context expires.
func waitForWindowsExporterHealth(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	pollInterval time.Duration,
) error {
	if ctx == nil {
		return fmt.Errorf("Windows Exporter health context is required")
	}
	if client == nil {
		return fmt.Errorf("Windows Exporter health HTTP client is required")
	}
	if strings.TrimSpace(endpoint) == "" {
		return fmt.Errorf("Windows Exporter health endpoint is required")
	}
	if pollInterval <= 0 {
		return fmt.Errorf("Windows Exporter health poll interval must be positive")
	}

	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("wait for Windows Exporter health: %w", err)
		}

		request, err := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			endpoint,
			nil,
		)
		if err != nil {
			return fmt.Errorf(
				"create Windows Exporter health request: %w",
				err,
			)
		}

		response, err := client.Do(request)
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()

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

			return fmt.Errorf(
				"wait for Windows Exporter health: %w",
				ctx.Err(),
			)

		case <-timer.C:
		}
	}
}
