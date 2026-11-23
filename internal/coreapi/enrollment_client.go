package coreapi

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	enrollment "github.com/ar-imms/telemetry-agent/internal/identity/enrollment/application"
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

// Enroll posts one immutable attempt to the trusted Core origin and validates
// the required success envelope and response fields before returning them.
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
	wireRequest := enrollmentRequestDTO{
		BootstrapToken: request.BootstrapToken,
		enrollmentPayloadDTO: enrollmentPayloadDTO{
			AttemptID:    request.AttemptID,
			Installation: installationDTO{ID: request.Installation.ID},
			Agent:        agentDTO{Version: request.Agent.Version},
			Environment: environmentDTO{
				OS: request.Environment.OS, Architecture: request.Environment.Architecture,
				Hostname: request.Environment.Hostname,
			},
			Metadata: request.Metadata,
		},
	}
	body, err := json.Marshal(wireRequest)
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
		var recordHeaderError tls.RecordHeaderError
		if errors.Is(err, http.ErrSchemeMismatch) || errors.As(err, &recordHeaderError) {
			return enrollment.EnrollmentResponse{}, &enrollment.RequestNotSentError{Code: "CORE_TLS_REQUIRED"}
		}
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
		Data enrollmentResponseDTO `json:"data"`
		Meta struct {
			RequestID string `json:"request_id"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		return enrollment.EnrollmentResponse{}, fmt.Errorf("decode Core enrollment response: %w", err)
	}
	if strings.TrimSpace(envelope.Meta.RequestID) == "" {
		return enrollment.EnrollmentResponse{}, fmt.Errorf("Core enrollment response is missing meta.request_id")
	}
	result := envelope.Data.applicationResponse()
	if err := result.Validate(request.Installation.ID, origin); err != nil {
		return enrollment.EnrollmentResponse{}, fmt.Errorf("validate Core enrollment response: %w", err)
	}
	return result, nil
}

// enrollmentRequestDTO and enrollmentResponseDTO keep Core's JSON contract at
// the protocol adapter boundary rather than in Identity domain models.
type enrollmentRequestDTO struct {
	BootstrapToken string `json:"bootstrap_token"`
	enrollmentPayloadDTO
}

type enrollmentPayloadDTO struct {
	AttemptID    string          `json:"enrollment_attempt_id"`
	Installation installationDTO `json:"installation"`
	Agent        agentDTO        `json:"agent"`
	Environment  environmentDTO  `json:"environment"`
	Metadata     map[string]any  `json:"metadata,omitempty"`
}

type installationDTO struct {
	ID string `json:"id"`
}

type agentDTO struct {
	Version string `json:"version"`
}

type environmentDTO struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	Hostname     string `json:"hostname,omitempty"`
}

type enrollmentResponseDTO struct {
	EnrollmentID   string          `json:"enrollment_id"`
	InstallationID string          `json:"installation_id"`
	Binding        bindingDTO      `json:"binding"`
	Credential     credentialDTO   `json:"credential"`
	ControlPlane   controlPlaneDTO `json:"control_plane"`
}

type bindingDTO struct {
	ID     string `json:"id"`
	NodeID string `json:"node_id"`
	Status string `json:"status"`
}

type credentialDTO struct {
	Type        string `json:"type"`
	AccessToken string `json:"access_token"`
}

type controlPlaneDTO struct {
	BaseURL             string `json:"base_url"`
	PollIntervalSeconds int    `json:"poll_interval_seconds"`
}

func (dto enrollmentResponseDTO) applicationResponse() enrollment.EnrollmentResponse {
	return enrollment.EnrollmentResponse{
		EnrollmentID: dto.EnrollmentID, InstallationID: dto.InstallationID,
		Binding:      enrollment.Binding{ID: dto.Binding.ID, NodeID: dto.Binding.NodeID, Status: dto.Binding.Status},
		Credential:   enrollment.Credential{Type: dto.Credential.Type, AccessToken: dto.Credential.AccessToken},
		ControlPlane: enrollment.ControlPlane{BaseURL: dto.ControlPlane.BaseURL, PollIntervalSeconds: dto.ControlPlane.PollIntervalSeconds},
	}
}

func classifyHTTPError(response *http.Response, body []byte) error {
	var problem struct {
		Status int    `json:"status"`
		Code   string `json:"code"`
	}
	_ = json.Unmarshal(body, &problem)
	code := strings.TrimSpace(problem.Code)
	if code == "" {
		code = "HTTP_" + strconv.Itoa(response.StatusCode)
	} else if problem.Status != response.StatusCode || !knownEnrollmentProblemCode(code, response.StatusCode) {
		code = "HTTP_" + strconv.Itoa(response.StatusCode)
	}
	// A gateway 502 can occur after Core commits but before its response reaches
	// the Agent. Keep the immutable attempt and retry it like other uncertain outcomes.
	if response.StatusCode == http.StatusBadGateway || response.StatusCode == http.StatusTooManyRequests || response.StatusCode == http.StatusServiceUnavailable {
		return &enrollment.RetryableError{Code: code, RetryAfter: parseRetryAfter(response.Header.Get("Retry-After"), time.Now())}
	}
	return &enrollment.PermanentError{Code: code}
}

func knownEnrollmentProblemCode(code string, status int) bool {
	wantStatus := map[string]int{
		"VALIDATION_ERROR": http.StatusBadRequest, "BAD_REQUEST": http.StatusBadRequest,
		"INVALID_BOOTSTRAP_TOKEN": http.StatusUnauthorized, "BOOTSTRAP_TOKEN_EXPIRED": http.StatusUnauthorized,
		"BOOTSTRAP_TOKEN_CONSUMED": http.StatusUnauthorized, "UNAUTHORIZED": http.StatusUnauthorized,
		"FORBIDDEN": http.StatusForbidden, "NOT_FOUND": http.StatusNotFound,
		"CONFLICT": http.StatusConflict, "NODE_NOT_ELIGIBLE": http.StatusConflict,
		"NODE_ALREADY_BOUND": http.StatusConflict, "INSTALLATION_ALREADY_BOUND": http.StatusConflict,
		"ENROLLMENT_ATTEMPT_CONFLICT": http.StatusConflict,
		"ENROLLMENT_RECOVERY_EXPIRED": http.StatusGone, "ENROLLMENT_RECOVERY_REVOKED": http.StatusGone,
		"TOO_MANY_REQUESTS": http.StatusTooManyRequests, "ENROLLMENT_UNAVAILABLE": http.StatusServiceUnavailable,
	}[code]
	return wantStatus != 0 && wantStatus == status
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
