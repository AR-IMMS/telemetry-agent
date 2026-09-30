//go:build !windows

package main

import (
	"context"
	"fmt"
	"io"
)

func runService(
	_ context.Context,
	_ []string,
	_ io.Writer,
	stderr io.Writer,
	_ dependencies,
) int {
	fmt.Fprintln(
		stderr,
		"usage error: service command is only supported on Windows",
	)

	return 2
}
