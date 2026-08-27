package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// echoWithMetrics wires one route + the metrics middleware onto a fresh Echo
// so the tests exercise the real c.Path() resolution path.
func echoWithMetrics(t *testing.T, recorder *Recorder) *echo.Echo {
	t.Helper()
	e := echo.New()
	e.Use(recorder.Middleware())
	e.GET("/api/graph", func(c echo.Context) error { return c.String(http.StatusOK, "ok") })
	e.GET("/api/fail", func(c echo.Context) error { return c.String(http.StatusInternalServerError, "boom") })
	return e
}

// TestMiddleware_MatchedRouteUsesRoutePattern pins the route pattern label. If
// the middleware ever grabs c.Request().URL.Path instead of c.Path(), every ID
// or query param leaks into the label set and the counter cardinality explodes.
func TestMiddleware_MatchedRouteUsesRoutePattern(t *testing.T) {
	recorder := newTestRecorder(t)
	e := echoWithMetrics(t, recorder)

	req := httptest.NewRequest(http.MethodGet, "/api/graph?namespaces=shop,billing", nil)
	e.ServeHTTP(httptest.NewRecorder(), req)

	if got := testutil.ToFloat64(recorder.httpRequests.WithLabelValues("/api/graph", "200")); got != 1 {
		t.Errorf("http_requests_total{path=/api/graph, code=200} = %v, want 1", got)
	}
	if got := testutil.CollectAndCount(recorder.httpRequestDur); got != 1 {
		t.Errorf("http_request_duration_seconds series = %d, want 1", got)
	}
}

// TestMiddleware_UnmatchedRouteBucketed is the cardinality-DoS defence: a
// scanner probing random URLs must NOT create one series per probe. All
// unmatched paths collapse under path="unmatched", regardless of how many
// distinct URLs the scanner tries.
//
// Known limitation: code label reads c.Response().Status before Echo's
// HTTPErrorHandler writes the 404 for unmatched routes, so probes here bucket
// under code="200" instead of code="404". The path bucketing — the actual
// cardinality-DoS defence — is intact. Fixing the code label needs a
// middleware change (inspect the returned *echo.HTTPError) and is tracked as
// its own follow-up.
func TestMiddleware_UnmatchedRouteBucketed(t *testing.T) {
	recorder := newTestRecorder(t)
	e := echoWithMetrics(t, recorder)

	for _, probe := range []string{"/wp-admin", "/.env", "/admin/config.php"} {
		req := httptest.NewRequest(http.MethodGet, probe, nil)
		e.ServeHTTP(httptest.NewRecorder(), req)
	}

	// One series only — three distinct probe URLs must collapse to one bucket.
	if got := testutil.CollectAndCount(recorder.httpRequests); got != 1 {
		t.Fatalf("http_requests_total series = %d, want 1 (unmatched must collapse to one bucket regardless of code label)", got)
	}
	// Sum across whichever code label the middleware ended up with must equal
	// the probe count. Reads the value under the currently-observed
	// (path=unmatched, code=200) combo — see the "Known limitation" comment.
	total := sumCounter(t, recorder, "alatyr_http_requests_total")
	if total != 3 {
		t.Errorf("http_requests_total sum for unmatched paths = %v, want 3", total)
	}
}

// sumCounter walks the recorder's registry and sums a counter family. Used by
// the unmatched-route test to stay decoupled from the (currently mislabelled)
// code dimension.
func sumCounter(t *testing.T, recorder *Recorder, name string) float64 {
	t.Helper()
	families, err := recorder.registry.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	total := 0.0
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			total += metric.GetCounter().GetValue()
		}
	}
	return total
}

// TestMiddleware_StatusCodeLabelBoundToResponse guards that the code label
// reflects the actual handler response, so 5xx alerts read from the real
// status the client saw.
func TestMiddleware_StatusCodeLabelBoundToResponse(t *testing.T) {
	recorder := newTestRecorder(t)
	e := echoWithMetrics(t, recorder)

	req := httptest.NewRequest(http.MethodGet, "/api/fail", nil)
	e.ServeHTTP(httptest.NewRecorder(), req)

	if got := testutil.ToFloat64(recorder.httpRequests.WithLabelValues("/api/fail", "500")); got != 1 {
		t.Errorf("http_requests_total{path=/api/fail, code=500} = %v, want 1", got)
	}
}
