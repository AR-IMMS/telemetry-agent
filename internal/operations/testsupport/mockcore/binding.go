package mockcore

import (
	"fmt"
	"strings"
)

// BrokerBindingSchemaV1 identifies the secret-free provisioning contract.
const BrokerBindingSchemaV1 = "ar-imms.operations.broker-binding.v1"

// BrokerBinding is the secret-free Core-to-Agent provisioning record for one
// installation binding. CredentialRef resolves only through protected storage.
type BrokerBinding struct {
	SchemaVersion string `json:"schema_version"`
	Installation  string `json:"installation_id"`
	BindingID     string `json:"binding_id"`
	Revision      uint64 `json:"revision"`
	Status        string `json:"status"`
	Endpoint      string `json:"endpoint,omitempty"`
	Username      string `json:"username,omitempty"`
	CredentialRef string `json:"credential_ref,omitempty"`
	CAFile        string `json:"ca_file,omitempty"`
	RequestStream string `json:"request_stream,omitempty"`
	Durable       string `json:"durable,omitempty"`
}

// ProvisionBrokerBinding replaces the Mock Core binding only with a newer,
// valid revision. Plaintext credentials are never accepted by this contract.
func (c *Core) ProvisionBrokerBinding(binding BrokerBinding) error {
	if c == nil || binding.SchemaVersion != BrokerBindingSchemaV1 || binding.Installation != c.installation ||
		!safeBindingToken(binding.BindingID) || binding.Revision == 0 ||
		(binding.Status != "active" && binding.Status != "revoked") {
		return fmt.Errorf("invalid Mock Core broker binding contract")
	}
	if binding.Status == "active" && (strings.TrimSpace(binding.Endpoint) == "" || strings.TrimSpace(binding.Username) == "" ||
		strings.TrimSpace(binding.CredentialRef) == "" || strings.TrimSpace(binding.CAFile) == "" ||
		!strings.HasPrefix(binding.Endpoint, "nats://") || !safeBindingToken(binding.RequestStream) || !safeBindingToken(binding.Durable)) {
		return fmt.Errorf("active Mock Core broker binding is incomplete")
	}
	if binding.Status == "revoked" && (binding.Endpoint != "" || binding.Username != "" || binding.CredentialRef != "" ||
		binding.CAFile != "" || binding.RequestStream != "" || binding.Durable != "") {
		return fmt.Errorf("revoked Mock Core broker binding must not retain broker credentials")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.bindings == nil {
		c.bindings = make(map[string]BrokerBinding)
	}
	if binding.Status == "active" {
		for bindingID, existing := range c.bindings {
			if bindingID != binding.BindingID && existing.Status == "active" &&
				(existing.Username == binding.Username || existing.CredentialRef == binding.CredentialRef) {
				return fmt.Errorf("active Mock Core broker bindings require distinct credentials")
			}
		}
	}
	if current, exists := c.bindings[binding.BindingID]; exists && binding.Revision <= current.Revision {
		return fmt.Errorf("Mock Core broker binding revision must increase")
	}
	c.bindings[binding.BindingID] = binding
	return nil
}

// BrokerBinding returns the current secret-free state of one binding.
func (c *Core) BrokerBinding(bindingID string) (BrokerBinding, bool) {
	if c == nil {
		return BrokerBinding{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	binding, exists := c.bindings[bindingID]
	return binding, exists
}

func safeBindingToken(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || character == '-' || character == '_') {
			return false
		}
	}
	return true
}
