package supervisor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"syscall"
	"time"
)

// VerifyReadiness checks the live Collector's generation token without
// starting another process. Callers must also confirm their supervised child
// remains alive around this probe.
func VerifyReadiness(ctx context.Context, endpoint, expectedBody string) error {
	if ctx == nil {
		return fmt.Errorf("readiness context is required")
	}
	if expectedBody == "" {
		return fmt.Errorf("generation-bound readiness token is required")
	}
	return checkReadiness(ctx, http.DefaultClient, endpoint, expectedBody)
}

func waitForReadiness(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	pollInterval time.Duration,
	expectedBody string,
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
		lastErr = checkReadiness(ctx, client, endpoint, expectedBody)
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
	expectedBody string,
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

	if response.StatusCode != http.StatusOK {
		// Treat every non-200 response as not ready, including otherwise successful HTTP responses.
		return fmt.Errorf(
			"readiness endpoint returned HTTP %d",
			response.StatusCode,
		)
	}
	if expectedBody == "" {
		if _, err := io.Copy(io.Discard, response.Body); err != nil {
			return fmt.Errorf("read readiness response body: %w", err)
		}
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, int64(len(expectedBody)+1)))
	if err != nil {
		return fmt.Errorf("read generation-bound readiness response: %w", err)
	}
	if string(body) != expectedBody {
		return fmt.Errorf("readiness response body did not match the launched generation")
	}

	return nil
}

func ensureReadinessEndpointVacant(ctx context.Context, endpoint string) error {
	parsed, err := url.ParseRequestURI(endpoint)
	if err != nil {
		return fmt.Errorf("parse health endpoint: %w", err)
	}
	host := parsed.Hostname()
	address := net.ParseIP(host)
	if host != "localhost" && (address == nil || !address.IsLoopback()) {
		return fmt.Errorf("generation-bound health endpoint must use loopback")
	}
	dialCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	connection, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", parsed.Host)
	if err == nil {
		_ = connection.Close()
		return fmt.Errorf("health listener is still serving before candidate launch")
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return nil
	}
	return fmt.Errorf("cannot prove that health listener is stopped: %w", err)
}
