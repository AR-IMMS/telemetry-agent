package mockcore

import "testing"

func TestBrokerBindingRevisionAndRevocationContract(t *testing.T) {
	core := &Core{installation: "11111111-2222-4333-8444-555555555555"}
	active := BrokerBinding{SchemaVersion: BrokerBindingSchemaV1, Installation: core.installation,
		BindingID: "binding-a", Revision: 1, Status: "active", Endpoint: "nats://broker:4222",
		Username: "agent-a", CredentialRef: "ops-binding-a", CAFile: "broker-ca.pem",
		RequestStream: "ARIMMS_OP_REQUESTS", Durable: "AGENT_A"}
	if err := core.ProvisionBrokerBinding(active); err != nil {
		t.Fatal(err)
	}
	if _, exists := core.BrokerBinding(active.BindingID); !exists {
		t.Fatal("provisioned binding was not retained")
	}
	otherBinding := active
	otherBinding.BindingID = "binding-b"
	otherBinding.Revision = 1
	if err := core.ProvisionBrokerBinding(otherBinding); err == nil {
		t.Fatal("two active bindings shared a broker username and credential reference")
	}
	stale := active
	if err := core.ProvisionBrokerBinding(stale); err == nil {
		t.Fatal("accepted a binding revision that did not advance")
	}
	rotated := active
	rotated.Revision = 2
	rotated.Username = "agent-a"
	rotated.CredentialRef = "ops-binding-a-rev2"
	if err := core.ProvisionBrokerBinding(rotated); err != nil {
		t.Fatal(err)
	}
	revoked := BrokerBinding{SchemaVersion: BrokerBindingSchemaV1, Installation: core.installation,
		BindingID: active.BindingID, Revision: 3, Status: "revoked"}
	if err := core.ProvisionBrokerBinding(revoked); err != nil {
		t.Fatal(err)
	}
	current, exists := core.BrokerBinding(active.BindingID)
	if !exists || current.Status != "revoked" || current.Revision != 3 || current.CredentialRef != "" {
		t.Fatalf("revoked binding retained active broker state: %+v", current)
	}
}
