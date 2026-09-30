package agenthealth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// FetchStatus retrieves the latest live Agent health snapshot.
func FetchStatus(
	ctx context.Context,
	client *http.Client,
	endpoint string,
) (Snapshot, error) {
	if ctx == nil {
		return Snapshot{}, fmt.Errorf("status request context is required")
	}
	if client == nil {
		return Snapshot{}, fmt.Errorf("status HTTP client is required")
	}

	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return Snapshot{}, fmt.Errorf("status endpoint is required")
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		endpoint,
		nil,
	)
	if err != nil {
		return Snapshot{}, fmt.Errorf("create status request: %w", err)
	}

	response, err := client.Do(request)
	if err != nil {
		return Snapshot{}, fmt.Errorf("request Agent status: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return Snapshot{}, fmt.Errorf(
			"request Agent status: unexpected HTTP status %s",
			response.Status,
		)
	}

	var snapshot Snapshot

	if err := json.NewDecoder(response.Body).Decode(&snapshot); err != nil {
		return Snapshot{}, fmt.Errorf(
			"decode Agent status response: %w",
			err,
		)
	}

	return snapshot, nil
}
