package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	nethttp "net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/identity/fleetenrollment/application"
	"github.com/ar-imms/telemetry-agent/internal/identity/fleetenrollment/domain"
)

const maxResponseBytes = 64 << 10

// NewClient creates an HTTP adapter using the caller's TLS and timeout policy.
// It rejects redirects so enrollment secrets cannot be forwarded to another endpoint.
func NewClient(client *nethttp.Client) application.Client {
	if client == nil {
		client = &nethttp.Client{Timeout: 15 * time.Second}
	}
	secureClient := *client
	secureClient.CheckRedirect = func(_ *nethttp.Request, _ []*nethttp.Request) error {
		return nethttp.ErrUseLastResponse
	}
	return httpClient{client: &secureClient}
}

type httpClient struct{ client *nethttp.Client }

// Exchange sends one immutable invitation exchange to Fleet.
func (c httpClient) Exchange(ctx context.Context, origin domain.Origin, request domain.ExchangeRequest) (domain.EnrollmentResponse, error) {
	var response domain.EnrollmentResponse
	if err := request.Validate(); err != nil {
		return response, fmt.Errorf("invalid Fleet exchange request")
	}
	return c.post(ctx, origin, "/v1/enrollment/exchanges", request, []int{nethttp.StatusCreated, nethttp.StatusOK})
}

// Recover retrieves only the original exchange result using the persisted proof.
func (c httpClient) Recover(ctx context.Context, origin domain.Origin, request domain.RecoveryRequest) (domain.EnrollmentResponse, error) {
	var response domain.EnrollmentResponse
	if _, err := domain.ParseOrigin(origin.String()); err != nil {
		return response, err
	}
	if request.AttemptID == "" || request.AgentID == "" || len(request.RecoveryProof) < 32 {
		return response, fmt.Errorf("invalid Fleet recovery request")
	}
	return c.post(ctx, origin, "/v1/enrollment/recoveries", request, []int{nethttp.StatusOK})
}

func (c httpClient) post(ctx context.Context, origin domain.Origin, path string, value any, successCodes []int) (domain.EnrollmentResponse, error) {
	var result domain.EnrollmentResponse
	body, err := json.Marshal(value)
	if err != nil {
		return result, fmt.Errorf("encode Fleet enrollment request")
	}
	request, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodPost, origin.String()+path, bytes.NewReader(body))
	if err != nil {
		return result, fmt.Errorf("create Fleet enrollment request")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, application/problem+json")
	response, err := c.client.Do(request)
	if err != nil {
		return result, fmt.Errorf("send Fleet enrollment request: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if !containsStatus(successCodes, response.StatusCode) {
		return result, decodeProblem(response)
	}
	if !hasNoStore(response.Header.Get("Cache-Control")) {
		return result, fmt.Errorf("fleet credential response must include Cache-Control: no-store")
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return domain.EnrollmentResponse{}, fmt.Errorf("decode Fleet enrollment response")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return domain.EnrollmentResponse{}, fmt.Errorf("fleet enrollment response contains trailing data")
	}
	return result, nil
}

func decodeProblem(response *nethttp.Response) error {
	var problem struct {
		Code string `json:"code"`
	}
	// Retain only the bounded, stable code; never include server detail in errors.
	_ = json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes)).Decode(&problem)
	code := safeProblemCode(problem.Code)
	retryAfter := time.Duration(0)
	if raw := strings.TrimSpace(response.Header.Get("Retry-After")); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
			retryAfter = time.Duration(seconds) * time.Second
		}
	}
	return ProblemError{Status: response.StatusCode, Code: code, RetryAfter: retryAfter}
}

func safeProblemCode(value string) string {
	code := strings.TrimSpace(value)
	if len(code) < 2 || len(code) > 64 || code[0] < 'A' || code[0] > 'Z' {
		return ""
	}
	for i := 0; i < len(code); i++ {
		if (code[i] < 'A' || code[i] > 'Z') && (code[i] < '0' || code[i] > '9') && code[i] != '_' {
			return ""
		}
	}
	return code
}

// ProblemError contains only sanitized response metadata.
type ProblemError struct {
	Status     int
	Code       string
	RetryAfter time.Duration
}

// OutcomeUncertain reports whether Fleet may have committed the exchange despite its response status.
func (e ProblemError) OutcomeUncertain() bool {
	return e.Status == nethttp.StatusRequestTimeout || e.Status == nethttp.StatusTooEarly ||
		e.Status == nethttp.StatusTooManyRequests || e.Status >= nethttp.StatusInternalServerError
}

// Error returns a safe status and optional machine code without response detail.
func (e ProblemError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("Fleet returned HTTP %d (%s)", e.Status, e.Code)
	}
	return fmt.Sprintf("Fleet returned HTTP %d", e.Status)
}

func containsStatus(values []int, status int) bool {
	for _, value := range values {
		if value == status {
			return true
		}
	}
	return false
}

func hasNoStore(value string) bool {
	for _, directive := range strings.Split(value, ",") {
		if strings.EqualFold(strings.TrimSpace(directive), "no-store") {
			return true
		}
	}
	return false
}
