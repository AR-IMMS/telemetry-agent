package processowner

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Options describes one executable and its non-secret command-line arguments.
type Options struct {
	Executable  string
	Arguments   []string
	Environment []string
}

func validate(options Options) error {
	if strings.TrimSpace(options.Executable) == "" {
		return fmt.Errorf("managed process executable is required")
	}
	if len(options.Arguments) == 0 {
		return fmt.Errorf("managed process arguments are required")
	}
	for _, argument := range options.Arguments {
		if strings.IndexByte(argument, 0) >= 0 {
			return fmt.Errorf("managed process arguments cannot contain NUL")
		}
	}
	for _, entry := range options.Environment {
		separator := strings.IndexByte(entry, '=')
		if separator < 1 || strings.IndexByte(entry, 0) >= 0 {
			return fmt.Errorf("managed process environment entries must have a valid name and value")
		}
	}
	return nil
}

func mergedEnvironment(base, overrides []string) []string {
	values := make(map[string]string, len(base)+len(overrides))
	for _, entry := range base {
		if key, ok := environmentKey(entry); ok {
			values[strings.ToUpper(key)] = entry
		}
	}
	for _, entry := range overrides {
		key, _ := environmentKey(entry)
		values[strings.ToUpper(key)] = entry
	}
	result := make([]string, 0, len(values))
	for _, entry := range values {
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool {
		left, _ := environmentKey(result[i])
		right, _ := environmentKey(result[j])
		return strings.ToUpper(left) < strings.ToUpper(right)
	})
	return result
}

func environmentKey(entry string) (string, bool) {
	separator := strings.IndexByte(entry, '=')
	if separator == 0 {
		separator = strings.IndexByte(entry[1:], '=') + 1
	}
	if separator <= 0 {
		return "", false
	}
	return entry[:separator], true
}

func processEnvironment(overrides []string) []string {
	if len(overrides) == 0 {
		return nil
	}
	return mergedEnvironment(os.Environ(), overrides)
}

// Run starts a process inside an OS-owned process group or Job Object. It
// cleans up contained descendants after cancellation or an unexpected exit.
func Run(ctx context.Context, options Options) error {
	if ctx == nil {
		return fmt.Errorf("managed process context is required")
	}
	if err := validate(options); err != nil {
		return err
	}
	return runContained(ctx, options)
}
