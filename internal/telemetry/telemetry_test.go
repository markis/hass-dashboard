package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
)

func TestInitDisabledLeavesNoopProviders(t *testing.T) {
	shutdown, err := Init(context.Background(), Config{})
	if err != nil {
		t.Fatalf("Init error: %v", err)
	}

	if shutdown == nil {
		t.Fatal("expected callable shutdown function when telemetry is disabled")
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

	_, span := otel.Tracer("github.com/markis/hass-dashboard/internal/telemetry/test").Start(
		context.Background(), "test.span")
	span.End()

	counter, err := otel.Meter("github.com/markis/hass-dashboard/internal/telemetry/test").
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
		want     string
	}{
		{
			name:     "environment overrides config",
			env:      "http://from-env:4318",
			endpoint: "http://from-config:4318",
			want:     "http://from-env:4318",
		},
		{
			name:     "config used when environment unset",
			env:      "",
			endpoint: "http://otel-collector:4318",
			want:     "http://otel-collector:4318",
		},
		{
			name:     "spec default when neither set",
			env:      "",
			endpoint: "",
			want:     defaultEndpoint,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != "" {
				t.Setenv(OTLPEnvEndpoint, tt.env)
			}

			got := resolveEndpoint(Config{Endpoint: tt.endpoint})
			if got != tt.want {
				t.Errorf("resolveEndpoint() = %q, want %q", got, tt.want)
			}
		})
	}
}
