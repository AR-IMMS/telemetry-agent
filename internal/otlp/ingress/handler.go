package ingress

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxOTLPBodyBytes = 4 << 20
const requestTimeout = 10 * time.Second

// Principal is the Fleet-authenticated identity and authorization for one request.
type Principal struct {
	AgentID   string
	Scope     string
	ExpiresAt time.Time
}

// Verifier resolves an opaque bearer against Fleet on every ingestion request.
// It returns an empty AgentID for a rejected credential and an error when Fleet
// is unavailable. Implementations must not cache positive results.
type Verifier interface {
	Verify(context.Context, string) (Principal, error)
}

type handler struct {
	verifier Verifier
	client   *http.Client
	gateway  string
}

// NewHandler creates the fail-closed public OTLP/HTTP JSON ingress.
// gatewayURL must point to the private, unauthenticated Collector receiver.
func NewHandler(verifier Verifier, gatewayURL string, client *http.Client) (http.Handler, error) {
	if verifier == nil || client == nil {
		return nil, fmt.Errorf("Fleet verifier and Gateway HTTP client are required")
	}
	parsed, err := url.Parse(gatewayURL)
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("internal Gateway URL must be an HTTP origin")
	}
	privateClient := *client
	privateClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return handler{verifier: verifier, client: &privateClient, gateway: strings.TrimSuffix(gatewayURL, "/")}, nil
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.TLS == nil {
		http.Error(w, "TLS required", http.StatusUpgradeRequired)
		return
	}
	if r.Method != http.MethodPost || signalField(r.URL.Path) == "" {
		http.NotFound(w, r)
		return
	}
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" || r.Header.Get("Content-Encoding") != "" {
		http.Error(w, "OTLP/HTTP JSON required", http.StatusUnsupportedMediaType)
		return
	}
	token, ok := bearerToken(r.Header.Values("Authorization"))
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	principal, err := h.verifier.Verify(ctx, token)
	if err != nil {
		http.Error(w, "credential authority unavailable", http.StatusServiceUnavailable)
		return
	}
	if principal.AgentID == "" || principal.ExpiresAt.Before(time.Now()) && !principal.ExpiresAt.IsZero() {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if principal.Scope != "otlp.write" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.ContentLength > maxOTLPBodyBytes {
		http.Error(w, "OTLP body too large", http.StatusRequestEntityTooLarge)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxOTLPBodyBytes+1))
	if err != nil || len(body) > maxOTLPBodyBytes {
		http.Error(w, "OTLP body too large", http.StatusRequestEntityTooLarge)
		return
	}
	clean, err := rewriteAgentID(body, signalField(r.URL.Path), principal.AgentID)
	if err != nil {
		http.Error(w, "invalid OTLP JSON", http.StatusBadRequest)
		return
	}
	forward, err := http.NewRequestWithContext(ctx, http.MethodPost, h.gateway+r.URL.Path, bytes.NewReader(clean))
	if err != nil {
		http.Error(w, "Gateway unavailable", http.StatusServiceUnavailable)
		return
	}
	forward.Header.Set("Content-Type", "application/json")
	response, err := h.client.Do(forward)
	if err != nil {
		http.Error(w, "Gateway unavailable", http.StatusServiceUnavailable)
		return
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		http.Error(w, "Gateway unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, "{}")
}

func signalField(path string) string {
	switch path {
	case "/v1/metrics":
		return "resourceMetrics"
	case "/v1/logs":
		return "resourceLogs"
	case "/v1/traces":
		return "resourceSpans"
	default:
		return ""
	}
}

func bearerToken(values []string) (string, bool) {
	if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") {
		return "", false
	}
	token := strings.TrimPrefix(values[0], "Bearer ")
	if len(token) < 32 || len(token) > 256 || strings.IndexFunc(token, func(r rune) bool {
		return r <= ' ' || r == ',' || r == ';'
	}) >= 0 {
		return "", false
	}
	return token, true
}

func rewriteAgentID(body []byte, field, agentID string) ([]byte, error) {
	var document map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF || document == nil {
		return nil, fmt.Errorf("OTLP JSON must contain one object")
	}
	resources, ok := document[field].([]any)
	if !ok || len(resources) == 0 {
		return nil, fmt.Errorf("OTLP resource list is missing")
	}
	scrubAgentIDs(document)
	for _, item := range resources {
		group, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("OTLP resource group is invalid")
		}
		resource, ok := group["resource"].(map[string]any)
		if !ok {
			resource = map[string]any{}
			group["resource"] = resource
		}
		attributes, _ := resource["attributes"].([]any)
		resource["attributes"] = append(attributes, map[string]any{"key": "agent_id", "value": map[string]any{"stringValue": agentID}})
	}
	return json.Marshal(document)
}

func scrubAgentIDs(root any) {
	stack := []any{root}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		switch value := current.(type) {
		case map[string]any:
			for key, child := range value {
				if key == "attributes" {
					if attributes, ok := child.([]any); ok {
						kept := make([]any, 0, len(attributes))
						for _, attribute := range attributes {
							entry, ok := attribute.(map[string]any)
							if !ok || entry["key"] != "agent_id" {
								kept = append(kept, attribute)
							}
						}
						value[key] = kept
						child = kept
					}
				}
				stack = append(stack, child)
			}
		case []any:
			stack = append(stack, value...)
		}
	}
}
