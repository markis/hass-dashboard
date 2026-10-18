// Package telemetry initializes the global OpenTelemetry SDK for the
// dashboard generator. When telemetry is disabled (the default) the global
// providers stay as the SDK's noop implementations, so instrumented call sites
// behave identically to un-instrumented builds.
//
// Configuration comes from the config.yaml `otel` section; the standard
// OTEL_EXPORTER_OTLP_ENDPOINT and OTEL_EXPORTER_OTLP_PROTOCOL environment
// variables take precedence over the file-based values to stay compatible
// with the OTLP spec.
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
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
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

	// OTLPEnvProtocol is the OTEL_EXPORTER_OTLP_PROTOCOL environment
	// variable, which overrides the config-file protocol when set.
	OTLPEnvProtocol = "OTEL_EXPORTER_OTLP_PROTOCOL"

	// ProtocolGRPC selects OTLP/gRPC export (collector port 4317).
	ProtocolGRPC = "grpc"

	// ProtocolHTTP selects OTLP/HTTP with protobuf payloads (collector port
	// 4318). It is the default protocol.
	ProtocolHTTP = "http/protobuf"

	// Default endpoints used when neither the config file nor the
	// environment specify one, per protocol (OTLP spec defaults).
	defaultGRPCEndpoint = "http://localhost:4317"
	defaultHTTPEndpoint = "http://localhost:4318"

	// Signal paths appended to the OTLP/HTTP base endpoint per the spec.
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
	// Endpoint is the OTLP base endpoint, e.g. "http://otel-collector:4318"
	// or "http://otel-collector:4317" for gRPC. An optional base path is
	// honored by OTLP/HTTP ("http://host:4318/collector"); traces and metrics
	// are sent to <base>/v1/traces and <base>/v1/metrics.
	// OTEL_EXPORTER_OTLP_ENDPOINT takes precedence when set.
	Endpoint string `yaml:"endpoint"`
	// Protocol selects the OTLP transport: "http/protobuf" (default) or
	// "grpc". OTEL_EXPORTER_OTLP_PROTOCOL takes precedence when set.
	Protocol string `yaml:"protocol"`
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

	protocol, err := resolveProtocol(cfg)
	if err != nil {
		return nil, err
	}

	endpoint := resolveEndpoint(cfg, protocol)

	host, basePath, insecure, err := splitEndpointURL(endpoint)
	if err != nil {
		return nil, err
	}

	traceExporter, metricExporter, err := buildExporters(ctx, protocol, host, basePath, insecure)
	if err != nil {
		return nil, err
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)

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

// buildExporters creates the trace and metric exporters for the chosen OTLP
// protocol. If the metric exporter fails after the trace one succeeded, the
// trace exporter is released before the error returns.
func buildExporters(
	ctx context.Context,
	protocol, host, basePath string,
	insecure bool,
) (sdktrace.SpanExporter, metric.Exporter, error) {
	switch protocol {
	case ProtocolGRPC:
		traceOptions := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(host)}
		metricOptions := []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpoint(host)}

		if insecure {
			traceOptions = append(traceOptions, otlptracegrpc.WithInsecure())
			metricOptions = append(metricOptions, otlpmetricgrpc.WithInsecure())
		}

		traceExporter, err := otlptracegrpc.New(ctx, traceOptions...)
		if err != nil {
			return nil, nil, fmt.Errorf("creating otlp/grpc trace exporter: %w", err)
		}

		metricExporter, err := otlpmetricgrpc.New(ctx, metricOptions...)
		if err != nil {
			releaseExporter(context.WithoutCancel(ctx), traceExporter, err)

			return nil, nil, fmt.Errorf("creating otlp/grpc metric exporter: %w", err)
		}

		return traceExporter, metricExporter, nil
	case ProtocolHTTP:
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
			return nil, nil, fmt.Errorf("creating otlp/http trace exporter: %w", err)
		}

		metricExporter, err := otlpmetrichttp.New(ctx, metricOptions...)
		if err != nil {
			releaseExporter(context.WithoutCancel(ctx), traceExporter, err)

			return nil, nil, fmt.Errorf("creating otlp/http metric exporter: %w", err)
		}

		return traceExporter, metricExporter, nil
	}

	return nil, nil, fmt.Errorf("unsupported otlp protocol %q: want \"grpc\" or \"http/protobuf\"", protocol)
}

// releaseExporter best-effort flushes a trace exporter when a later init step
// failed and the exporter will not get a provider.
func releaseExporter(ctx context.Context, exporter sdktrace.SpanExporter, cause error) {
	if err := exporter.Shutdown(ctx); err != nil {
		log.Printf("Warning: failed to release exporter after %v: %v", cause, err)
	}
}

// resolveProtocol returns the OTLP transport protocol in precedence order:
// environment, config file, then the HTTP default.
func resolveProtocol(cfg Config) (string, error) {
	if protocol := os.Getenv(OTLPEnvProtocol); protocol != "" {
		return validProtocol(protocol)
	}

	if cfg.Protocol != "" {
		return validProtocol(cfg.Protocol)
	}

	return ProtocolHTTP, nil
}

// validProtocol accepts the protocols the OTLP Go exporters implement.
func validProtocol(protocol string) (string, error) {
	switch protocol {
	case ProtocolGRPC, ProtocolHTTP:
		return protocol, nil
	}

	return "", fmt.Errorf("unsupported otlp protocol %q: want \"grpc\" or \"http/protobuf\"", protocol)
}

// resolveEndpoint returns the OTLP endpoint to export to, in precedence order:
// environment, config file, then the spec default for the protocol.
func resolveEndpoint(cfg Config, protocol string) string {
	if endpoint := os.Getenv(OTLPEnvEndpoint); endpoint != "" {
		return endpoint
	}

	if cfg.Endpoint != "" {
		return cfg.Endpoint
	}

	if protocol == ProtocolGRPC {
		return defaultGRPCEndpoint
	}

	return defaultHTTPEndpoint
}

// splitEndpointURL parses an OTLP base endpoint of the form
// scheme://host[:port]/[base-path] into exporter option constituents.
// The scheme selects transport security (anything other than https is
// insecure), mirroring how the exporters treat OTEL_EXPORTER_OTLP_ENDPOINT.
// The base path is only used by OTLP/HTTP; gRPC routes by method name.
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
