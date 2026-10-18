// Package telemetry initializes the global OpenTelemetry SDK for the
// dashboard generator. When telemetry is disabled (the default) the global
// providers stay as the SDK's noop implementations, so instrumented call sites
// behave identically to un-instrumented builds.
//
// Configuration comes from the config.yaml `otel` section; the standard
// OTEL_EXPORTER_OTLP_ENDPOINT environment variable takes precedence over the
// file-based endpoint to stay compatible with the OTLP spec.
package telemetry

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"path"
	"runtime/debug"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const (
	// OTLPEnvEndpoint is the OTEL_EXPORTER_OTLP_ENDPOINT environment
	// variable, which overrides the config-file endpoint when set.
	OTLPEnvEndpoint = "OTEL_EXPORTER_OTLP_ENDPOINT"

	// defaultEndpoint is the OTLP/HTTP collector endpoint used when neither
	// the config file nor the environment specify one.
	defaultEndpoint = "http://localhost:4318"

	// Signal paths appended to the base endpoint per the OTLP/HTTP spec.
	tracesPath  = "/v1/traces"
	metricsPath = "/v1/metrics"

	// serviceName is the resource attribute identifying this service.
	serviceName = "hass-dashboard"
)

// Config controls OpenTelemetry export.
type Config struct {
	// Enabled turns telemetry on. When false the noop global providers are
	// left in place.
	Enabled bool `yaml:"enabled"`
	// Endpoint is the OTLP/HTTP base endpoint, e.g. "http://otel-collector:4318".
	// An optional base path is allowed ("http://host:4318/collector"); traces
	// and metrics are sent to <base>/v1/traces and <base>/v1/metrics. The
	// standard OTEL_EXPORTER_OTLP_ENDPOINT environment variable takes
	// precedence over this value.
	Endpoint string `yaml:"endpoint"`
}

// ShutdownFunc flushes and releases the telemetry providers. When telemetry
// is disabled the returned function is a no-op.
type ShutdownFunc func(ctx context.Context) error

// Init configures the global OpenTelemetry tracer and meter providers per cfg.
// It must be called before any instrumented components are constructed.
func Init(ctx context.Context, cfg Config) (ShutdownFunc, error) {
	if !cfg.Enabled {
		return func(_ context.Context) error {
			return nil
		}, nil
	}

	res, err := resource.Merge(resource.Default(), resource.NewSchemaless(
		attribute.String("service.name", serviceName),
		attribute.String("service.version", serviceVersion()),
	))
	if err != nil {
		return nil, fmt.Errorf("building otel resource: %w", err)
	}

	endpoint := resolveEndpoint(cfg)

	host, basePath, insecure, err := splitEndpointURL(endpoint)
	if err != nil {
		return nil, err
	}

	traceOptions := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(host),
		otlptracehttp.WithURLPath(path.Join(basePath, tracesPath)),
	}

	metricOptions := []otlpmetrichttp.Option{
		otlpmetrichttp.WithEndpoint(host),
		otlpmetrichttp.WithURLPath(path.Join(basePath, metricsPath)),
	}

	if insecure {
		traceOptions = append(traceOptions, otlptracehttp.WithInsecure())
		metricOptions = append(metricOptions, otlpmetrichttp.WithInsecure())
	}

	traceExporter, err := otlptracehttp.New(ctx, traceOptions...)
	if err != nil {
		return nil, fmt.Errorf("creating otlp trace exporter: %w", err)
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)

	metricExporter, err := otlpmetrichttp.New(ctx, metricOptions...)
	if err != nil {
		if closeErr := tracerProvider.Shutdown(context.WithoutCancel(ctx)); closeErr != nil {
			log.Printf("Warning: failed to shut down tracer provider: %v", closeErr)
		}

		return nil, fmt.Errorf("creating otlp metric exporter: %w", err)
	}

	meterProvider := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(metricExporter)),
		metric.WithResource(res),
	)

	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	return func(shutdownCtx context.Context) error {
		if err := tracerProvider.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutting down tracer provider: %w", err)
		}

		if err := meterProvider.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutting down meter provider: %w", err)
		}

		return nil
	}, nil
}

// resolveEndpoint returns the OTLP endpoint to export to, in precedence order:
// environment, config file, then the spec default.
func resolveEndpoint(cfg Config) string {
	if endpoint := os.Getenv(OTLPEnvEndpoint); endpoint != "" {
		return endpoint
	}

	if cfg.Endpoint != "" {
		return cfg.Endpoint
	}

	return defaultEndpoint
}

// splitEndpointURL parses an OTLP base endpoint of the form
// scheme://host[:port]/[base-path] into exporter option constituents.
// The scheme selects transport security (anything other than https is
// insecure), mirroring how the exporters treat OTEL_EXPORTER_OTLP_ENDPOINT.
func splitEndpointURL(endpoint string) (string, string, bool, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", "", false, fmt.Errorf("parsing otlp endpoint %q: %w", endpoint, err)
	}

	if parsed.Host == "" || parsed.Scheme == "" {
		return "", "", false, fmt.Errorf("parsing otlp endpoint %q: want scheme://host[:port]", endpoint)
	}

	return parsed.Host, strings.TrimSuffix(parsed.Path, "/"), parsed.Scheme != "https", nil
}

// serviceVersion returns the best available version string: the module
// version from build info, falling back to the VCS revision, then "dev".
func serviceVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}

	if info.Main.Version != "" {
		return info.Main.Version
	}

	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			return setting.Value
		}
	}

	return "dev"
}

// ShutdownTimeout is the context timeout used when flushing telemetry on exit.
const ShutdownTimeout = 5 * time.Second
