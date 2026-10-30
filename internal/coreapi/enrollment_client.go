package coreapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/enrollment"
)

const (
	enrollmentEndpoint = "/api/v1/agent-enrollments"
	maxResponseBytes   = 1024 * 1024
	defaultHTTPTimeout = 15 * time.Second
)

// HTTPEnrollmentClient sends enrollment requests over HTTPS to a configured
// Core origin. Redirects are never followed, preventing secret forwarding.
type HTTPEnrollmentClient struct {
	client *http.Client
}

// NewEnrollmentClient creates a bounded HTTP adapter. A supplied client's
// transport and TLS settings are retained, but redirects are always disabled.
func NewEnrollmentClient() *HTTPEnrollmentClient {
	return newEnrollmentClient(&http.Client{})
}

func newEnrollmentClient(client *http.Client) *HTTPEnrollmentClient {
	if client == nil {
		client = &http.Client{}
	}
	clientCopy := *client
	if clientCopy.Timeout <= 0 {
		clientCopy.Timeout = defaultHTTPTimeout
	}
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &HTTPEnrollmentClient{client: &clientCopy}
}

// Enroll posts one immutable attempt to the trusted Core origin. It returns
// only a decoded response; the application layer validates identity and URL
// trust before persisting the binding or issued credential.
func (c *HTTPEnrollmentClient) Enroll(
	ctx context.Context,
	origin enrollment.CoreOrigin,
	request enrollment.EnrollmentRequest,
) (enrollment.EnrollmentResponse, error) {
	if ctx == nil {
		return enrollment.EnrollmentResponse{}, fmt.Errorf("enrollment request context is required")
	}
	if c == nil || c.client == nil {
		return enrollment.EnrollmentResponse{}, fmt.Errorf("Core HTTP client is required")
	}
	base, err := url.Parse(origin.String())
	if err != nil || base.Scheme != "https" || base.Hostname() == "" {
		return enrollment.EnrollmentResponse{}, fmt.Errorf("trusted HTTPS Core origin is required")
	}
	base.Path = enrollmentEndpoint
	base.RawPath, base.RawQuery, base.Fragment = "", "", ""
	body, err := json.Marshal(request)
	if err != nil {
		return enrollment.EnrollmentResponse{}, fmt.Errorf("encode enrollment request: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, base.String(), bytes.NewReader(body))
	if err != nil {
		return enrollment.EnrollmentResponse{}, fmt.Errorf("create Core enrollment request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json, application/problem+json")
	response, err := c.client.Do(httpRequest)
	if err != nil {
		return enrollment.EnrollmentResponse{}, fmt.Errorf("perform Core enrollment request: %w", err)
	}
	defer response.Body.Close()
	content, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return enrollment.EnrollmentResponse{}, fmt.Errorf("read Core enrollment response: %w", err)
	}
	if len(content) > maxResponseBytes {
		return enrollment.EnrollmentResponse{}, fmt.Errorf("Core enrollment response exceeds 1 MiB")
	}
	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK {
		return enrollment.EnrollmentResponse{}, classifyHTTPError(response, content)
	}
	var envelope struct {
		Data enrollment.EnrollmentResponse `json:"data"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		return enrollment.EnrollmentResponse{}, fmt.Errorf("decode Core enrollment response: %w", err)
	}
	return envelope.Data, nil
}

func classifyHTTPError(response *http.Response, body []byte) error {
	var problem struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(body, &problem)
	code := strings.TrimSpace(problem.Code)
	if code == "" {
		code = "HTTP_" + strconv.Itoa(response.StatusCode)
	} else if !knownEnrollmentProblemCode(code) {
		code = "HTTP_" + strconv.Itoa(response.StatusCode)
	}
	if response.StatusCode == http.StatusTooManyRequests || response.StatusCode == http.StatusRequestTimeout || response.StatusCode >= 500 {
		return &enrollment.RetryableError{Code: code, RetryAfter: parseRetryAfter(response.Header.Get("Retry-After"), time.Now())}
	}
	return &enrollment.PermanentError{Code: code}
}

func knownEnrollmentProblemCode(code string) bool {
	switch code {
	case "INVALID_ENROLLMENT_PAYLOAD", "INVALID_BOOTSTRAP_TOKEN", "BOOTSTRAP_TOKEN_EXPIRED",
		"BOOTSTRAP_TOKEN_CONSUMED", "NODE_ALREADY_BOUND", "INSTALLATION_ALREADY_BOUND",
		"ENROLLMENT_ATTEMPT_CONFLICT", "RATE_LIMIT_EXCEEDED", "ENROLLMENT_UNAVAILABLE":
		return true
	default:
		return false
	}
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	date, err := http.ParseTime(value)
	if err != nil || !date.After(now) {
		return 0
	}
	return date.Sub(now)
}
