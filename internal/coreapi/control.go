package coreapi

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/coreconfig"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

const agentManagementPath = "/api/v1/agent-management"

// AgentManagementAPI reads the authenticated installation policy and reports
// Supervisor configuration status. It does not own enrollment or credentials.
type AgentManagementAPI interface {
	GetSession(context.Context) (AgentManagementSession, error)
	BindOpAMPInstance(context.Context, string) (string, error)
	ReportStatus(context.Context, SupervisorStatusReport) error
}

// DesiredGatewayPolicy is the only remote policy in the v1 control contract.
type DesiredGatewayPolicy struct {
	SchemaVersion   string `json:"schema_version"`
	Revision        int64  `json:"revision"`
	GatewayEndpoint string `json:"gateway_endpoint"`
}

// AgentManagementSession is resolved from the authenticated enrollment bearer.
type AgentManagementSession struct {
	InstallationID   string               `json:"installation_id"`
	NodeID           string               `json:"node_id"`
	OpAMPInstanceUID string               `json:"opamp_instance_uid"`
	DesiredPolicy    DesiredGatewayPolicy `json:"desired_policy"`
}

// SupervisorStatusReport contains protocol-observed config state only.
type SupervisorStatusReport struct {
	InstanceUID     string `json:"instance_uid"`
	DesiredRevision int64  `json:"desired_revision"`
	AppliedRevision int64  `json:"applied_revision,omitempty"`
	Status          string `json:"status"`
	ConfigHash      string `json:"config_hash"`
	Message         string `json:"message,omitempty"`
}

// ControlAPIError reports a safe HTTP status and contract code without body data.
type ControlAPIError struct {
	Status int
	Code   string
}

func (e *ControlAPIError) Error() string {
	return fmt.Sprintf("Core agent management request failed: status=%d code=%s", e.Status, e.Code)
}

// AgentManagementClient calls the proposed versioned Core management contract.
type AgentManagementClient struct {
	client *http.Client
	base   *url.URL
	token  string
}

// NewAgentManagementClient creates an HTTPS-only client using the existing
// enrollment bearer. Redirects are rejected so credentials cannot be forwarded.
func NewAgentManagementClient(baseURL, accessToken string, client *http.Client) (*AgentManagementClient, error) {
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || base.Scheme != "https" || base.Hostname() == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, fmt.Errorf("trusted HTTPS Core URL is required")
	}
	if strings.TrimSpace(accessToken) == "" || strings.TrimSpace(accessToken) != accessToken {
		return nil, fmt.Errorf("enrollment bearer credential is required")
	}
	if client == nil {
		client = &http.Client{}
	}
	copy := *client
	if copy.Timeout <= 0 {
		copy.Timeout = defaultHTTPTimeout
	}
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &AgentManagementClient{client: &copy, base: base, token: accessToken}, nil
}

// GetSession retrieves policy and identity bound to the authenticated bearer.
func (c *AgentManagementClient) GetSession(ctx context.Context) (AgentManagementSession, error) {
	var envelope struct {
		Data AgentManagementSession `json:"data"`
		Meta struct {
			RequestID string `json:"request_id"`
		} `json:"meta"`
	}
	if err := c.request(ctx, http.MethodGet, "/session", nil, http.StatusOK, &envelope); err != nil {
		return AgentManagementSession{}, err
	}
	if strings.TrimSpace(envelope.Meta.RequestID) == "" {
		return AgentManagementSession{}, fmt.Errorf("Core agent management response is missing meta.request_id")
	}
	if err := validateAgentManagementSession(envelope.Data); err != nil {
		return AgentManagementSession{}, err
	}
	return envelope.Data, nil
}

// BindOpAMPInstance sets or confirms the Supervisor UID on the authenticated
// installation. A conflicting existing UID is returned as a contract error.
func (c *AgentManagementClient) BindOpAMPInstance(ctx context.Context, uid string) (string, error) {
	if _, err := identity.ParseInstallationID(uid); err != nil {
		return "", fmt.Errorf("OpAMP instance UID is invalid")
	}
	var envelope struct {
		Data struct {
			InstanceUID string `json:"instance_uid"`
		} `json:"data"`
		Meta struct {
			RequestID string `json:"request_id"`
		} `json:"meta"`
	}
	if err := c.request(ctx, http.MethodPut, "/opamp-instance", map[string]string{"instance_uid": uid}, http.StatusOK, &envelope); err != nil {
		return "", err
	}
	if strings.TrimSpace(envelope.Meta.RequestID) == "" || envelope.Data.InstanceUID != uid {
		return "", fmt.Errorf("Core OpAMP binding response does not match the requested instance")
	}
	return envelope.Data.InstanceUID, nil
}

// ReportStatus sends an idempotent, sanitized protocol status observation.
func (c *AgentManagementClient) ReportStatus(ctx context.Context, report SupervisorStatusReport) error {
	if _, err := identity.ParseInstallationID(report.InstanceUID); err != nil || report.DesiredRevision < 1 || report.AppliedRevision < 0 || report.AppliedRevision > report.DesiredRevision || !validRemoteConfigStatus(report.Status) || len(report.ConfigHash) != 64 || strings.ToLower(report.ConfigHash) != report.ConfigHash {
		return fmt.Errorf("Supervisor status report is invalid")
	}
	if _, err := hex.DecodeString(report.ConfigHash); err != nil {
		return fmt.Errorf("Supervisor status config hash must be lowercase SHA-256 hex")
	}
	if len(report.Message) > 512 || strings.ContainsAny(report.Message, "\r\n\x00") {
		return fmt.Errorf("Supervisor status message is invalid")
	}
	return c.request(ctx, http.MethodPut, "/status", report, http.StatusNoContent, nil)
}

func (c *AgentManagementClient) request(ctx context.Context, method, suffix string, body any, wantStatus int, output any) error {
	if ctx == nil || c == nil || c.client == nil || c.base == nil || c.token == "" {
		return fmt.Errorf("Core agent management client and context are required")
	}
	endpoint := *c.base
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + agentManagementPath + suffix
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode Core agent management request: %w", err)
		}
		reader = strings.NewReader(string(encoded))
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), reader)
	if err != nil {
		return fmt.Errorf("create Core agent management request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Accept", "application/json, application/problem+json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.client.Do(request)
	if err != nil {
		return fmt.Errorf("perform Core agent management request: %w", err)
	}
	defer response.Body.Close()
	content, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(content) > maxResponseBytes {
		return fmt.Errorf("read Core agent management response")
	}
	if response.StatusCode != wantStatus {
		return controlAPIError(response.StatusCode, content)
	}
	if output != nil {
		if err := json.Unmarshal(content, output); err != nil {
			return fmt.Errorf("decode Core agent management response: %w", err)
		}
	}
	return nil
}

func controlAPIError(status int, body []byte) error {
	var problem struct {
		Status int    `json:"status"`
		Code   string `json:"code"`
	}
	_ = json.Unmarshal(body, &problem)
	if problem.Status != status || strings.TrimSpace(problem.Code) == "" {
		problem.Code = "HTTP_ERROR"
	}
	return &ControlAPIError{Status: status, Code: problem.Code}
}

func validateAgentManagementSession(value AgentManagementSession) error {
	if _, err := identity.ParseInstallationID(value.InstallationID); err != nil {
		return fmt.Errorf("Core agent management response has an invalid installation ID")
	}
	if _, err := identity.ParseInstallationID(value.NodeID); err != nil {
		return fmt.Errorf("Core agent management response has an invalid Node ID")
	}
	if value.OpAMPInstanceUID != "" {
		if _, err := identity.ParseInstallationID(value.OpAMPInstanceUID); err != nil {
			return fmt.Errorf("Core agent management response has an invalid OpAMP instance UID")
		}
	}
	if value.DesiredPolicy.SchemaVersion != "ar-imms/gateway-policy/v1" || value.DesiredPolicy.Revision < 1 {
		return fmt.Errorf("Core agent management response has an unsupported policy revision")
	}
	if err := coreconfig.ValidateGatewayEndpoint(value.DesiredPolicy.GatewayEndpoint); err != nil {
		return fmt.Errorf("Core agent management response has an invalid Gateway endpoint")
	}
	return nil
}

func validRemoteConfigStatus(status string) bool {
	return status == "APPLYING" || status == "APPLIED" || status == "FAILED"
}

var _ AgentManagementAPI = (*AgentManagementClient)(nil)
