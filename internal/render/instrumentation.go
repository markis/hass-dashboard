package render

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// renderMeterName is the instrumentation scope name for this package.
const renderMeterName = "github.com/markis/hass-dashboard/internal/render"

// tracer returns this package's instrumentation scope tracer, resolved from
// the global provider (noop until telemetry.Init enables the SDK).
func tracer() trace.Tracer {
	return otel.Tracer(renderMeterName)
}

// meter returns this package's meter from the global provider.
func meter() otelmetric.Meter {
	return otel.Meter(renderMeterName)
}

// recordError attaches err to the active span as an error event and marks the
// span's status as errored.
func recordError(span trace.Span, err error) {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}
