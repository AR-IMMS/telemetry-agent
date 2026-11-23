package agentlifecycle

import (
	runtimecontracts "github.com/ar-imms/telemetry-agent/internal/runtime/collector/contracts"
	"github.com/ar-imms/telemetry-agent/internal/runtime/collector/supervisor"
)

func (w runtimeWatcher) launchOptions() supervisor.Options {
	options := w.options
	if reporter, ok := w.healthReporter.(runtimecontracts.CollectorOutputReporter); ok {
		options.OnOutput = reporter.ReportCollectorOutput
	}
	return options
}
