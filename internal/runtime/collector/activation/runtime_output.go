package agentlifecycle

import (
	agenthealth "github.com/ar-imms/telemetry-agent/internal/health"
	"github.com/ar-imms/telemetry-agent/internal/runtime/collector/supervisor"
)

func (w runtimeWatcher) launchOptions() supervisor.Options {
	options := w.options
	if reporter, ok := w.healthReporter.(agenthealth.CollectorOutputReporter); ok {
		options.OnOutput = reporter.ReportCollectorOutput
	}
	return options
}
