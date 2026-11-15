package enrollment

import "testing"

func TestIdentityEnrollmentRejectsInsecureCoreOrigin(t *testing.T) {
	if _, err := ParseCoreOrigin("http://core.example.test"); err == nil {
		t.Fatal("ParseCoreOrigin accepted an insecure Core origin")
	}
}
