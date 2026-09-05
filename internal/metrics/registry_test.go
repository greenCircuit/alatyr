package metrics

import (
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"alatyr/internal/models"
)

// TestNew_DefaultConfigLeavesOptInVecsNil pins the always-off default for the
// high-cardinality per-workload vecs. Register-once semantics mean flipping the
// default silently doubles /metrics body size on the first restart.
func TestNew_DefaultConfigLeavesOptInVecsNil(t *testing.T) {
	recorder := New(Config{}, nil)

	if recorder.workloadStatus != nil {
		t.Error("workloadStatus registered under default config — must be gated behind DetailWorkload")
	}
	if recorder.workloadIssues != nil {
		t.Error("workloadIssues registered under default config — must be gated behind DetailWorkload")
	}
	// IssuePolicyBreakdown defaults to on (documented in Config.IssuePolicyBreakdownEnabled).
	if recorder.issuesByPolicy == nil {
		t.Error("issuesByPolicy nil under default config — breakdown defaults to enabled")
	}
	if recorder.policyLayeringByPolicy == nil {
		t.Error("policyLayeringByPolicy nil under default config — same gate as issuesByPolicy")
	}
}

// TestNew_DetailWorkloadRegistersOptInVecs and TestRecordWorkloads_EmitsPerWorkload
// together prove the DetailWorkload path actually works end-to-end when the
// operator opts in. Without this a config-flip bug lands silently — vecs nil,
// scrape misses the entire per-workload family, no test catches it.
func TestNew_DetailWorkloadRegistersOptInVecs(t *testing.T) {
	recorder := New(Config{Detail: DetailWorkload}, nil)

	if recorder.workloadStatus == nil {
		t.Fatal("workloadStatus nil despite Detail=DetailWorkload")
	}
	if recorder.workloadIssues == nil {
		t.Fatal("workloadIssues nil despite Detail=DetailWorkload")
	}
}

func TestRecordWorkloads_DetailWorkloadEmitsPerWorkload(t *testing.T) {
	recorder := New(Config{Detail: DetailWorkload}, nil)
	workload := models.WorkloadNode{
		ID: "uid-a", Label: "checkout", Namespace: "shop", Type: models.NodeTypeDeployment,
		Statuses: []models.StatusKey{models.StatusInternetIngress},
	}
	cache := &models.Cache{
		NsIndex: map[string]models.NSIndex{"shop": {Workloads: []models.WorkloadNode{workload}}},
	}
	recorder.recordWorkloads(cache, []string{"k8s"})

	if got := testutil.ToFloat64(recorder.workloadStatus.WithLabelValues("shop", "checkout", string(models.StatusInternetIngress))); got != 1 {
		t.Errorf("workload_status{shop, checkout, internet-ingress} = %v, want 1", got)
	}
}

// TestHandler_ServesPrometheusText covers the /metrics wire path directly.
// promhttp negotiates content-type; anything else is a downstream scrape
// failure that the endpoint smoke tests cannot reach without a live process.
func TestHandler_ServesPrometheusText(t *testing.T) {
	recorder := New(Config{}, nil)
	recorder.SetBuildInfo("v0.0.0-test", "abc123")
	req := httptest.NewRequest("GET", "/metrics", nil)
	resp := httptest.NewRecorder()

	recorder.Handler().ServeHTTP(resp, req)

	if resp.Code != 200 {
		t.Fatalf("Handler served %d, want 200", resp.Code)
	}
	body := resp.Body.String()
	if !strings.Contains(body, "# HELP alatyr_build_info") {
		t.Errorf("Handler body missing alatyr_build_info declaration; first 200 chars:\n%s", body[:min(200, len(body))])
	}
	if !strings.Contains(body, `alatyr_build_info{commit="abc123"`) &&
		!strings.Contains(body, `commit="abc123"`) {
		t.Errorf("Handler body missing commit label; first 400 chars:\n%s", body[:min(400, len(body))])
	}
}

// TestRecorder_ConcurrentSnapshotAndMiddleware runs the snapshot loop against
// the HTTP middleware under -race. If either write path grows an unprotected
// map or slice, this trips instead of shipping a flaky prod panic. client_golang
// vec ops are documented as safe, but the surrounding recordX code walks caller
// data — a future refactor introducing a shared map is exactly the failure
// mode this guards.
func TestRecorder_ConcurrentSnapshotAndMiddleware(t *testing.T) {
	recorder := newTestRecorder(t)
	cache, engines := snapshotFixtureCache()

	var waitGroup sync.WaitGroup
	stop := make(chan struct{})

	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		for {
			select {
			case <-stop:
				return
			default:
				recorder.RecordSnapshot(cache, nil, engines)
			}
		}
	}()

	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		for i := 0; i < 200; i++ {
			recorder.httpRequests.WithLabelValues("/api/graph", "200").Inc()
			recorder.httpRequestDur.WithLabelValues("/api/graph").Observe(0.01)
		}
	}()

	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		for i := 0; i < 50; i++ {
			req := httptest.NewRequest("GET", "/metrics", nil)
			resp := httptest.NewRecorder()
			recorder.Handler().ServeHTTP(resp, req)
			if resp.Code != 200 {
				t.Errorf("scrape %d returned %d", i, resp.Code)
				return
			}
		}
		close(stop)
	}()

	waitGroup.Wait()
}
