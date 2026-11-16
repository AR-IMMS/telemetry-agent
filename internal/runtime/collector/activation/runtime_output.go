package agentlifecycle

import (
	"github.com/ar-imms/telemetry-agent/internal/agenthealth"
	"github.com/ar-imms/telemetry-agent/internal/supervisor"
)

func (w runtimeWatcher) launchOptions() supervisor.Options {
	options := w.options
	if reporter, ok := w.healthReporter.(agenthealth.CollectorOutputReporter); ok {
		options.OnOutput = reporter.ReportCollectorOutput
	}
	return options
}
