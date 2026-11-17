package redis

import definitions "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/domain"
import redisdefinition "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/redis"

const redisMetricsCapability = "otel.receiver.redis.metrics"

// Definition is retained as a source-compatible forwarding entry to Monitoring's Redis profile.
func Definition() definitions.Definition { return redisdefinition.Definition() }
