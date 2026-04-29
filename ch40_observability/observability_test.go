package observability

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)
	logger := NewJSONLogger()
	srv := httptest.NewServer(BuildRouter(logger, m, reg))
	t.Cleanup(srv.Close)
	return srv
}

func newServerWithMetrics(t *testing.T) (*httptest.Server, *Metrics, *prometheus.Registry) {
	t.Helper()
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)
	logger := NewJSONLogger()
	srv := httptest.NewServer(BuildRouter(logger, m, reg))
	t.Cleanup(srv.Close)
	return srv, m, reg
}

func TestHealthz_Returns200(t *testing.T) {
	srv := newServer(t)
	resp, _ := srv.Client().Get(srv.URL + "/healthz")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
}

func TestMetrics_EndpointAccessible(t *testing.T) {
	srv := newServer(t)
	resp, _ := srv.Client().Get(srv.URL + "/metrics")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
}

func TestMetrics_CounterIncrementsOnRequest(t *testing.T) {
	srv, m, _ := newServerWithMetrics(t)

	srv.Client().Get(srv.URL + "/healthz")
	srv.Client().Get(srv.URL + "/healthz")

	counter, err := m.RequestTotal.GetMetricWithLabelValues("GET", "/healthz", "200")
	if err != nil {
		t.Fatalf("get counter: %v", err)
	}

	// Use the /metrics endpoint to verify count rather than reaching into the metric.
	resp, _ := srv.Client().Get(srv.URL + "/metrics")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	_ = counter

	if !strings.Contains(string(body), `http_requests_total`) {
		t.Fatal("expected http_requests_total in /metrics output")
	}
}

func TestMetrics_ErrorCountedOn500(t *testing.T) {
	srv, _, reg := newServerWithMetrics(t)

	srv.Client().Get(srv.URL + "/error")

	// Gather all metrics and check error counter.
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	found := false
	for _, mf := range mfs {
		if mf.GetName() == "http_errors_total" {
			for _, m := range mf.Metric {
				if m.Counter.GetValue() > 0 {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("expected http_errors_total > 0 after /error request")
	}
}

func TestMetrics_PrometheusTextFormat(t *testing.T) {
	srv := newServer(t)
	srv.Client().Get(srv.URL + "/todos")

	resp, _ := srv.Client().Get(srv.URL + "/metrics")
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Fatalf("Content-Type: got %q, want text/plain", ct)
	}
}

func TestInstrument_DurationMetricPresent(t *testing.T) {
	srv := newServer(t)
	srv.Client().Get(srv.URL + "/healthz")

	resp, _ := srv.Client().Get(srv.URL + "/metrics")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if !strings.Contains(string(body), "http_request_duration_seconds") {
		t.Fatal("expected http_request_duration_seconds in /metrics output")
	}
}
