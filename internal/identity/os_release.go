package identity

import "strings"

type osRelease struct {
	Distribution string
	Version      string
}

func parseOSRelease(input []byte) osRelease {
	// Parse only simple KEY=VALUE entries and let later entries override earlier ones.
	values := make(map[string]string)
	for _, line := range strings.Split(string(input), "\n") {
		key, value, found := strings.Cut(line, "=")
		if !found || strings.TrimSpace(key) == "" {
			continue
		}
		values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"`)
	}
	return osRelease{Distribution: values["ID"], Version: values["VERSION_ID"]}
}
