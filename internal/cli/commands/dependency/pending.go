package dependency

import (
	"fmt"
	"io"
	"sort"
)

// PendingTeardown is the CLI projection of one dependency teardown awaiting
// Collector convergence.
type PendingTeardown struct {
	Name       string
	Action     string
	Generation uint64
}

// PendingTeardowns writes the existing pending-teardown presentation in name
// order and returns its established exit code.
func PendingTeardowns(stdout io.Writer, pending []PendingTeardown, appliedGeneration uint64) int {
	if len(pending) == 0 {
		fmt.Fprintln(stdout, "No pending dependency teardowns.")
		return 0
	}

	sort.Slice(pending, func(i, j int) bool { return pending[i].Name < pending[j].Name })
	fmt.Fprintln(stdout, "Pending dependency teardowns:")
	for _, item := range pending {
		fmt.Fprintf(stdout, "- %s: %s, generation %d (applied generation: %d)\n", item.Name, item.Action, item.Generation, appliedGeneration)
	}
	return 0
}
