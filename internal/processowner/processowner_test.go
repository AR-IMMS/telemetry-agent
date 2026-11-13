package processowner

import "testing"

func TestValidateRequiresAnExecutableAndArguments(t *testing.T) {
	if err := validate(Options{}); err == nil {
		t.Fatal("empty process options were accepted")
	}
	if err := validate(Options{Executable: "supervisor"}); err == nil {
		t.Fatal("missing config argument was accepted")
	}
	if err := validate(Options{Executable: "supervisor", Arguments: []string{"--config=supervisor.yaml"}}); err != nil {
		t.Fatalf("valid process options: %v", err)
	}
}
