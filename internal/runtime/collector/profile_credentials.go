package collectorruntime

import (
	"os"
	"strings"
)

// ProfileCredentialsConfigured reports whether referenced PostgreSQL environment values exist.
// Values are checked locally and never returned, logged, or persisted by Runtime.
func ProfileCredentialsConfigured(receiverType, usernameEnv, passwordEnv string) bool {
	if receiverType != "postgresql" {
		return true
	}
	username, usernameOK := os.LookupEnv(usernameEnv)
	password, passwordOK := os.LookupEnv(passwordEnv)
	return usernameOK && strings.TrimSpace(username) != "" && passwordOK && strings.TrimSpace(password) != ""
}
