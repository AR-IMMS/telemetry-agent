package supervisor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

func waitForReadiness(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	pollInterval time.Duration,
) error {
	// Poll until the bounded startup context expires; transient health failures are expected.
	if client == nil {
		return fmt.Errorf("readiness HTTP client is required")
	}
	if endpoint == "" {
		return fmt.Errorf("readiness endpoint is required")
	}
	if pollInterval <= 0 {
		return fmt.Errorf("readiness poll interval must be positive")
	}

	var lastErr error

	for {
		lastErr = checkReadiness(ctx, client, endpoint)
		if lastErr == nil {
			// HTTP 200 is the Collector readiness contract; no extra startup delay is needed.
			return nil
		}

		timer := time.NewTimer(pollInterval)

		select {
		case <-ctx.Done():
			// Stop the timer before returning to avoid leaving a timer active after cancellation.
			if !timer.Stop() {
				<-timer.C
			}

			return fmt.Errorf(
				"wait for Collector readiness: %w; last check: %v",
				ctx.Err(),
				lastErr,
			)

		case <-timer.C:
			// Retry only after the configured interval to avoid a tight failure loop.
		}
	}
}

func checkReadiness(
	ctx context.Context,
	client *http.Client,
	endpoint string,
) error {
	// Drain the response body so the HTTP client can reuse connections during polling.
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		endpoint,
		nil,
	)
	if err != nil {
		return fmt.Errorf("create readiness request: %w", err)
	}

	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("request readiness endpoint: %w", err)
	}
	defer response.Body.Close()

	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		return fmt.Errorf("read readiness response body: %w", err)
	}

	if response.StatusCode != http.StatusOK {
		// Treat every non-200 response as not ready, including otherwise successful HTTP responses.
		return fmt.Errorf(
			"readiness endpoint returned HTTP %d",
			response.StatusCode,
		)
	}

	return nil
}
