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

	"github.com/google/uuid"
)

const fleetIntrospectionPath = "/internal/v1/otlp/credentials/introspect"
const maxFleetResponseBytes = 4096

// FleetVerifier resolves each opaque OTLP bearer through Fleet without caching.
// Its HTTP client must be configured with the Gateway's Fleet client certificate
// and the Fleet server trust roots by the composition root.
type FleetVerifier struct {
	origin string
	client *http.Client
}

// NewFleetVerifier accepts only an HTTPS Fleet origin and disables redirects.
func NewFleetVerifier(origin string, client *http.Client) (*FleetVerifier, error) {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		(parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" || client == nil {
		return nil, fmt.Errorf("Fleet introspection requires an HTTPS origin and client")
	}
	privateClient := *client
	privateClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &FleetVerifier{origin: strings.TrimSuffix(origin, "/"), client: &privateClient}, nil
}

// Verify asks Fleet for the current credential state on every request.
// An inactive credential returns an empty principal. Errors represent an
// unavailable or invalid authority response and must fail ingestion closed.
func (v *FleetVerifier) Verify(ctx context.Context, token string) (Principal, error) {
	if v == nil || v.client == nil || ctx == nil || len(token) < 32 || len(token) > 256 {
		return Principal{}, fmt.Errorf("Fleet verification request is invalid")
	}
	body, err := json.Marshal(struct {
		Token string `json:"token"`
	}{Token: token})
	if err != nil {
		return Principal{}, fmt.Errorf("encode Fleet verification request")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, v.origin+fleetIntrospectionPath, bytes.NewReader(body))
	if err != nil {
		return Principal{}, fmt.Errorf("create Fleet verification request")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := v.client.Do(request)
	if err != nil {
		return Principal{}, fmt.Errorf("Fleet verification unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Principal{}, fmt.Errorf("Fleet verification returned an unusable response")
	}
	contentType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" || !hasNoStore(response.Header.Get("Cache-Control")) {
		return Principal{}, fmt.Errorf("Fleet verification response is not protected")
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, maxFleetResponseBytes+1))
	if err != nil || len(content) > maxFleetResponseBytes {
		return Principal{}, fmt.Errorf("Fleet verification response is invalid")
	}
	var result struct {
		SchemaVersion int        `json:"schema_version"`
		Active        bool       `json:"active"`
		AgentID       string     `json:"agent_id"`
		CredentialID  string     `json:"credential_id"`
		Scope         string     `json:"scope"`
		ExpiresAt     *time.Time `json:"expires_at"`
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil || result.SchemaVersion != 1 {
		return Principal{}, fmt.Errorf("Fleet verification response is invalid")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return Principal{}, fmt.Errorf("Fleet verification response has trailing data")
	}
	if !result.Active {
		return Principal{}, nil
	}
	if _, err := uuid.Parse(result.AgentID); err != nil || result.CredentialID == "" || result.Scope == "" {
		return Principal{}, fmt.Errorf("Fleet verification identity is invalid")
	}
	principal := Principal{AgentID: result.AgentID, Scope: result.Scope}
	if result.ExpiresAt != nil {
		principal.ExpiresAt = *result.ExpiresAt
	}
	return principal, nil
}

func hasNoStore(value string) bool {
	for _, directive := range strings.Split(value, ",") {
		if strings.EqualFold(strings.TrimSpace(directive), "no-store") {
			return true
		}
	}
	return false
}
