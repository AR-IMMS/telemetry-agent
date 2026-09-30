package agenthealth

import (
	"encoding/json"
	"net/http"
)

// NewHTTPHandler returns the read-only local status API handler.
func NewHTTPHandler(provider Provider) http.Handler {
	return http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.URL.Path != "/v1/status" {
			http.NotFound(response, request)

			return
		}

		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			http.Error(
				response,
				"method not allowed",
				http.StatusMethodNotAllowed,
			)

			return
		}

		if provider == nil {
			http.Error(
				response,
				"health provider is unavailable",
				http.StatusServiceUnavailable,
			)

			return
		}

		response.Header().Set(
			"Content-Type",
			"application/json; charset=utf-8",
		)

		if err := json.NewEncoder(response).Encode(provider.Snapshot()); err != nil {
			http.Error(
				response,
				"encode health snapshot",
				http.StatusInternalServerError,
			)
		}
	})
}
