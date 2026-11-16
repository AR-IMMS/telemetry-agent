package inventory

import "testing"

func TestCycleResultSucceededUsesProviderOutcomes(t *testing.T) {
	if !(CycleResult{}).Succeeded() {
		t.Fatal("empty configured provider set should retain successful-cycle semantics")
	}
	if !(CycleResult{ProviderResults: []ProviderResult{{ProviderID: "docker", ProviderInstanceID: "local"}}}).Succeeded() {
		t.Fatal("successful provider cycle reported failure")
	}
	if (CycleResult{ProviderResults: []ProviderResult{
		{ProviderID: "docker", ProviderInstanceID: "local"},
		{ProviderID: "docker", ProviderInstanceID: "remote", ErrorCode: "daemon_unavailable"},
	}}).Succeeded() {
		t.Fatal("cycle with a failed provider reported success")
	}
}
