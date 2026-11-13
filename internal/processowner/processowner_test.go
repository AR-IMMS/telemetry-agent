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
	if err := validate(Options{Executable: "supervisor", Arguments: []string{"--config=supervisor.yaml"}, Environment: []string{"BROKEN"}}); err == nil {
		t.Fatal("malformed environment entry was accepted")
	}
}

func TestMergedEnvironmentReplacesNamesCaseInsensitively(t *testing.T) {
	got := mergedEnvironment([]string{"PATH=old", "KEEP=value"}, []string{"Path=new"})
	if len(got) != 2 {
		t.Fatalf("merged environment = %v, want two entries", got)
	}
	if got[0] != "KEEP=value" || got[1] != "Path=new" {
		t.Fatalf("merged environment = %v", got)
	}
}
