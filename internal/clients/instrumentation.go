package clients

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// tracer returns this package's instrumentation scope tracer, resolved from
// the global provider (noop until telemetry.Init enables the SDK).
func tracer() trace.Tracer {
	return otel.Tracer("github.com/markis/hass-dashboard/internal/clients")
}
