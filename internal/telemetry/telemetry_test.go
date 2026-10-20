package telemetry

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	colmetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
)

func TestInitDisabledLeavesNoopProviders(t *testing.T) {
	shutdown, err := Init(context.Background(), Config{})
	if err != nil {
		t.Fatalf("Init error: %v", err)
	}

	if err := shutdown(context.Background()); err != nil {
		t.Errorf("disabled shutdown should be a no-op, got: %v", err)
	}
}

func TestInitEnabledExportsTracesAndMetrics(t *testing.T) {
	paths := make(chan string, 16)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case paths <- r.URL.Path:
		default: // never block the collector's retries
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	shutdown, err := Init(context.Background(), Config{Enabled: true, Endpoint: server.URL})
	if err != nil {
		t.Fatalf("Init error: %v", err)
	}

	if shutdown == nil {
		t.Fatal("expected non-nil shutdown function when telemetry is enabled")
	}

	_, span := otel.Tracer("github.com/markis/home-dashboard/internal/telemetry/test").Start(
		context.Background(), "test.span")
	span.End()

	counter, err := otel.Meter("github.com/markis/home-dashboard/internal/telemetry/test").
		Int64Counter("dashboard.test.count")
	if err != nil {
		t.Fatalf("creating counter: %v", err)
	}

	counter.Add(context.Background(), 1)

	if err := shutdown(context.Background()); err != nil {
		t.Errorf("shutdown: %v", err)
	}

	sawTraces, sawMetrics := false, false

	deadline := time.After(2 * time.Second)

	for !sawTraces || !sawMetrics {
		select {
		case path := <-paths:
			switch {
			case strings.HasSuffix(path, "/v1/traces"):
				sawTraces = true
			case strings.HasSuffix(path, "/v1/metrics"):
				sawMetrics = true
			}
		case <-deadline:
			t.Fatalf("missing exports after shutdown: traces=%v metrics=%v", sawTraces, sawMetrics)
		}
	}
}

// TestInitGRPCExportsTracesAndMetrics verifies the OTLP/gRPC path end to end
// against an in-process stub collector serving the real collector services.
func TestInitGRPCExportsTracesAndMetrics(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening: %v", err)
	}

	traceCollector := &traceCollectorServer{}
	metricsCollector := &metricsCollectorServer{}

	grpcServer := grpc.NewServer()

	coltracev1.RegisterTraceServiceServer(grpcServer, traceCollector)
	colmetricsv1.RegisterMetricsServiceServer(grpcServer, metricsCollector)

	go grpcServer.Serve(listener)
	defer grpcServer.Stop()

	shutdown, err := Init(context.Background(), Config{
		Enabled:  true,
		Protocol: ProtocolGRPC,
		Endpoint: "http://" + listener.Addr().String(),
	})
	if err != nil {
		t.Fatalf("Init error: %v", err)
	}

	_, span := otel.Tracer("github.com/markis/home-dashboard/internal/telemetry/test").Start(
		context.Background(), "test.span")
	span.End()

	counter, err := otel.Meter("github.com/markis/home-dashboard/internal/telemetry/test").
		Int64Counter("dashboard.test.count")
	if err != nil {
		t.Fatalf("creating counter: %v", err)
	}

	counter.Add(context.Background(), 1)

	if err := shutdown(context.Background()); err != nil {
		t.Errorf("shutdown: %v", err)
	}

	sawTraces, sawMetrics := int(traceCollector.exports.Load()), int(metricsCollector.exports.Load())

	deadline := time.Now().Add(2 * time.Second)
	for (sawTraces == 0 || sawMetrics == 0) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)

		sawTraces = int(traceCollector.exports.Load())
		sawMetrics = int(metricsCollector.exports.Load())
	}

	if sawTraces == 0 {
		t.Error("no trace exports reached the stub collector")
	}

	if sawMetrics == 0 {
		t.Error("no metric exports reached the stub collector")
	}
}

// traceCollectorServer is a stub OTLP collector counting trace Export calls.
type traceCollectorServer struct {
	coltracev1.UnimplementedTraceServiceServer

	exports atomic.Int64
}

func (c *traceCollectorServer) Export(
	_ context.Context,
	req *coltracev1.ExportTraceServiceRequest,
) (*coltracev1.ExportTraceServiceResponse, error) {
	c.exports.Add(int64(len(req.GetResourceSpans())))

	return &coltracev1.ExportTraceServiceResponse{}, nil
}

// metricsCollectorServer is a stub OTLP collector counting metric Export calls.
type metricsCollectorServer struct {
	colmetricsv1.UnimplementedMetricsServiceServer

	exports atomic.Int64
}

func (c *metricsCollectorServer) Export(
	_ context.Context,
	req *colmetricsv1.ExportMetricsServiceRequest,
) (*colmetricsv1.ExportMetricsServiceResponse, error) {
	c.exports.Add(int64(len(req.GetResourceMetrics())))

	return &colmetricsv1.ExportMetricsServiceResponse{}, nil
}

func TestResolveProtocol(t *testing.T) {
	tests := []struct {
		name     string
		env      string
		protocol string
		want     string
		wantErr  bool
	}{
		{name: "environment overrides config", env: ProtocolGRPC, protocol: ProtocolHTTP, want: ProtocolGRPC},
		{name: "config used when environment unset", protocol: ProtocolGRPC, want: ProtocolGRPC},
		{name: "spec default when neither set", env: "", protocol: "", want: ProtocolHTTP},
		{name: "unsupported env", env: "http/json", wantErr: true},
		{name: "unsupported config", protocol: "json", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != "" {
				t.Setenv(OTLPEnvProtocol, tt.env)
			}

			got, err := resolveProtocol(Config{Protocol: tt.protocol})
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveProtocol() = %q, want error", got)
				}

				return
			}

			if err != nil {
				t.Fatalf("resolveProtocol() error: %v", err)
			}

			if got != tt.want {
				t.Errorf("resolveProtocol() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSplitEndpointURL(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		host     string
		basePath string
		insecure bool
		wantErr  bool
	}{
		{
			name:     "host and port, insecure",
			endpoint: "http://otel-collector:4318",
			host:     "otel-collector:4318",
			basePath: "",
			insecure: true,
		},
		{
			name:     "https is secure",
			endpoint: "https://collector.example.com:4318/prefix",
			host:     "collector.example.com:4318",
			basePath: "/prefix",
			insecure: false,
		},
		{
			name:     "trailing slash dropped",
			endpoint: "http://otel-collector:4318/",
			host:     "otel-collector:4318",
			basePath: "",
			insecure: true,
		},
		{
			name:     "missing scheme rejected",
			endpoint: "otel-collector:4318",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, basePath, insecure, err := splitEndpointURL(tt.endpoint)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("splitEndpointURL(%q) = nil error, want error", tt.endpoint)
				}

				return
			}

			if err != nil {
				t.Fatalf("splitEndpointURL(%q) error: %v", tt.endpoint, err)
			}

			if host != tt.host {
				t.Errorf("host = %q, want %q", host, tt.host)
			}

			if basePath != tt.basePath {
				t.Errorf("basePath = %q, want %q", basePath, tt.basePath)
			}

			if insecure != tt.insecure {
				t.Errorf("insecure = %v, want %v", insecure, tt.insecure)
			}
		})
	}
}

func TestResolveEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		env      string
		endpoint string
		protocol string
		want     string
	}{
		{
			name:     "environment overrides config",
			env:      "http://from-env:4318",
			endpoint: "http://from-config:4318",
			protocol: ProtocolHTTP,
			want:     "http://from-env:4318",
		},
		{
			name:     "config used when environment unset",
			env:      "",
			endpoint: "http://otel-collector:4318",
			protocol: ProtocolHTTP,
			want:     "http://otel-collector:4318",
		},
		{
			name:     "grpc spec default when neither set",
			env:      "",
			endpoint: "",
			protocol: ProtocolGRPC,
			want:     defaultGRPCEndpoint,
		},
		{
			name:     "http spec default when neither set",
			env:      "",
			endpoint: "",
			protocol: ProtocolHTTP,
			want:     defaultHTTPEndpoint,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != "" {
				t.Setenv(OTLPEnvEndpoint, tt.env)
			}

			got := resolveEndpoint(Config{Endpoint: tt.endpoint}, tt.protocol)
			if got != tt.want {
				t.Errorf("resolveEndpoint() = %q, want %q", got, tt.want)
			}
		})
	}
}
