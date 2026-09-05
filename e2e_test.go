package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"alatyr/internal/models"
)

// End-to-end coverage for the -f manifest path: the binary must produce a
// report from a scenario dir, and must serve the same cluster over HTTP.
// Scenarios are the spec — every expectation below is written against the
// header comment in test-data/scenarios/<name>/cluster.yaml.

const scenarioRoot = "test-data/scenarios"

type reportDocument struct {
	Target string `json:"target"`
	Counts struct {
		Workloads              int `json:"workloads"`
		Manifests              int `json:"manifests"`
		CoverageWithGlobal     int `json:"coverageWithGlobal"`
		CoverageWithoutGlobal  int `json:"coverageWithoutGlobal"`
		NoPolicyWithGlobals    int `json:"noPolicyWithGlobals"`
		NoPolicyWithoutGlobals int `json:"noPolicyWithoutGlobals"`
	} `json:"counts"`
	BreakingIssuesCount struct {
		TotalIssues int                          `json:"totalIssues"`
		ByType      map[models.IssueType]int     `json:"byType"`
		BySeverity  map[models.IssueSeverity]int `json:"bySeverity"`
	} `json:"breakingIssuesCount"`
	IssuesCount struct {
		TotalIssues         int                      `json:"totalIssues"`
		TotalIssuesBreaking int                      `json:"totalIssuesBreaking"`
		ByType              map[models.IssueType]int `json:"byType"`
	} `json:"issuesCount"`
	MeshCount struct {
		NsEnrolled int `json:"nsEnrolled"`
	} `json:"meshCount"`
	Issues                   []models.Issue                       `json:"issues"`
	NoCoverageWithGlobals    []struct{ Node models.WorkloadNode } `json:"noCoverageWithGlobals"`
	NoCoverageWithoutGlobals []struct{ Node models.WorkloadNode } `json:"noCoverageWithoutGlobals"`
}

type graphDocument struct {
	Nodes []models.WorkloadNode `json:"nodes"`
	Edges []struct {
		Source       string `json:"source"`
		Target       string `json:"target"`
		PolicySource string `json:"policySource"`
	} `json:"edges"`
}

type clusterStateDocument struct {
	AvailableNs   []string `json:"availableNs"`
	StatusKeys    []string `json:"statusKeys"`
	PolicySources []string `json:"policySources"`
}

// buildBinary compiles the server once per test binary run. Every e2e case
// shells out to this path.
func buildBinary(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "graph")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Env = append(os.Environ(), "GOTOOLCHAIN=auto")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, output)
	}
	return binary
}

func runReport(t *testing.T, binary, scenario string, extraArgs ...string) (reportDocument, string) {
	t.Helper()
	args := append([]string{"--report", "-f", filepath.Join(scenarioRoot, scenario), "--format", "json"}, extraArgs...)
	command := exec.Command(binary, args...)
	var stderr strings.Builder
	command.Stderr = &stderr
	stdout, err := command.Output()
	if err != nil {
		t.Fatalf("%s --report failed: %v\nstderr:\n%s", scenario, err, stderr.String())
	}

	var document reportDocument
	if err := json.Unmarshal(stdout, &document); err != nil {
		t.Fatalf("%s: stdout is not a JSON report (logs must go to stderr): %v\n%s", scenario, err, stdout)
	}
	return document, stderr.String()
}

// The report is the CI-facing document: it must parse, count only real
// workloads, and split coverage with and without the cluster-scoped engine.
func TestE2EReportPerScenario(t *testing.T) {
	binary := buildBinary(t)

	cases := []struct {
		scenario   string
		workloads  int
		nsEnrolled int
		// coverage counted with globals, then without
		coveredWithGlobal    int
		coveredWithoutGlobal int
		uncoveredIDs         []string // uncovered by any engine, per the fixture header
		wantBreakingTypes    []models.IssueType
	}{
		{
			scenario: "01-k8s-baseline", workloads: 5,
			coveredWithGlobal: 4, coveredWithoutGlobal: 4,
			uncoveredIDs: []string{"scratch"},
		},
		{
			scenario: "02-istio-l7", workloads: 4, nsEnrolled: 3,
			coveredWithGlobal: 4, coveredWithoutGlobal: 4,
		},
		{
			scenario: "03-mtls-matrix", workloads: 5, nsEnrolled: 1,
			coveredWithGlobal: 3, coveredWithoutGlobal: 3,
			wantBreakingTypes: []models.IssueType{models.MeshConflicts, models.MeshTransportBlocked},
		},
		{
			// Calico GNP covers proxy; only the global engine does, so the
			// without-globals column must drop by one.
			scenario: "04-calico-globals", workloads: 3,
			coveredWithGlobal: 3, coveredWithoutGlobal: 2,
			wantBreakingTypes: []models.IssueType{models.PolicyConflicts, models.IssuesCidrScope},
		},
		{
			// deployment, statefulset, daemonset, cronjob, job, bare pod —
			// the Succeeded pod must not appear.
			scenario: "05-workload-kinds", workloads: 6,
			coveredWithGlobal: 6, coveredWithoutGlobal: 6,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.scenario, func(t *testing.T) {
			document, stderr := runReport(t, binary, testCase.scenario)

			if document.Target != filepath.Join(scenarioRoot, testCase.scenario) {
				t.Errorf("target = %q", document.Target)
			}
			if document.Counts.Workloads != testCase.workloads {
				t.Errorf("workloads = %d, want %d", document.Counts.Workloads, testCase.workloads)
			}
			if document.Counts.Manifests == 0 {
				t.Error("manifests = 0, want the scenario's cluster.yaml counted")
			}
			if document.MeshCount.NsEnrolled != testCase.nsEnrolled {
				t.Errorf("nsEnrolled = %d, want %d", document.MeshCount.NsEnrolled, testCase.nsEnrolled)
			}

			if document.Counts.CoverageWithGlobal != testCase.coveredWithGlobal ||
				document.Counts.CoverageWithoutGlobal != testCase.coveredWithoutGlobal {
				t.Errorf("coverage with/without globals = %d/%d, want %d/%d",
					document.Counts.CoverageWithGlobal, document.Counts.CoverageWithoutGlobal,
					testCase.coveredWithGlobal, testCase.coveredWithoutGlobal)
			}
			if document.Counts.CoverageWithGlobal+document.Counts.NoPolicyWithGlobals != testCase.workloads {
				t.Errorf("covered+uncovered != workloads: %d+%d vs %d",
					document.Counts.CoverageWithGlobal, document.Counts.NoPolicyWithGlobals, testCase.workloads)
			}
			if len(document.NoCoverageWithGlobals) != document.Counts.NoPolicyWithGlobals {
				t.Errorf("uncovered list len %d != count %d",
					len(document.NoCoverageWithGlobals), document.Counts.NoPolicyWithGlobals)
			}
			for _, wantID := range testCase.uncoveredIDs {
				found := false
				for _, missing := range document.NoCoverageWithGlobals {
					if strings.Contains(missing.Node.ID, wantID) || missing.Node.Label == wantID {
						found = true
					}
				}
				if !found {
					t.Errorf("expected %q in the uncovered list, got %+v", wantID, document.NoCoverageWithGlobals)
				}
			}

			for _, wantType := range testCase.wantBreakingTypes {
				if document.BreakingIssuesCount.ByType[wantType] == 0 {
					t.Errorf("no %q findings; byType = %v", wantType, document.BreakingIssuesCount.ByType)
				}
			}
			if document.IssuesCount.TotalIssuesBreaking != document.BreakingIssuesCount.TotalIssues {
				t.Errorf("breaking tallies disagree: %d vs %d",
					document.IssuesCount.TotalIssuesBreaking, document.BreakingIssuesCount.TotalIssues)
			}
			// Info/secure findings must never reach the CI gate count.
			for _, severity := range []models.IssueSeverity{models.IssueSeverityInfo, models.IssueSeveritySecure} {
				if document.BreakingIssuesCount.BySeverity[severity] != 0 {
					t.Errorf("%q counted as breaking", severity)
				}
			}
			if len(document.Issues) != document.IssuesCount.TotalIssues {
				t.Errorf("issues len %d != totalIssues %d", len(document.Issues), document.IssuesCount.TotalIssues)
			}
			// Every known detector must be seeded so a consumer can tell
			// "nothing fired" from "detector removed".
			for issueType := range models.SeverityByType {
				if _, ok := document.IssuesCount.ByType[issueType]; !ok {
					t.Errorf("issuesCount.byType missing %q", issueType)
				}
			}
			if !strings.Contains(stderr, "populate cache done") && stderr != "" && strings.Contains(stderr, "{") {
				t.Errorf("unexpected stderr shape:\n%s", stderr)
			}
		})
	}
}

// --output writes the same document the table run summarizes, and the default
// (table) format stays human-readable on stdout.
func TestE2EReportOutputFileAndTableFormat(t *testing.T) {
	binary := buildBinary(t)
	scenario := filepath.Join(scenarioRoot, "03-mtls-matrix")
	jsonPath := filepath.Join(t.TempDir(), "report.json")

	command := exec.Command(binary, "--report", "-f", scenario, "--output", jsonPath)
	stdout, err := command.Output()
	if err != nil {
		t.Fatalf("report failed: %v", err)
	}

	table := string(stdout)
	for _, want := range []string{"manifests scan", "COVERAGE", "SEVERITY", "findings ("} {
		if !strings.Contains(table, want) {
			t.Errorf("table output missing %q:\n%s", want, table)
		}
	}

	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("--output wrote no file: %v", err)
	}
	var document reportDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("--output file is not valid JSON: %v", err)
	}
	if !strings.Contains(table, fmt.Sprintf("%d workloads", document.Counts.Workloads)) {
		t.Errorf("table and JSON disagree on workload count (%d):\n%s", document.Counts.Workloads, table)
	}
	if document.BreakingIssuesCount.ByType[models.MeshConflicts] == 0 {
		t.Errorf("03-mtls-matrix must report a mesh conflict; byType = %v", document.BreakingIssuesCount.ByType)
	}
}

// Bad invocations must exit non-zero rather than print an empty report.
func TestE2EReportInvalidInvocations(t *testing.T) {
	binary := buildBinary(t)
	scenario := filepath.Join(scenarioRoot, "01-k8s-baseline")

	cases := map[string][]string{
		"report without -f": {"--report"},
		"missing dir":       {"--report", "-f", filepath.Join(t.TempDir(), "nope")},
		"unknown format":    {"--report", "-f", scenario, "--format", "yaml"},
		"unwritable output": {"--report", "-f", scenario, "--output", filepath.Join(t.TempDir(), "missing-dir", "report.json")},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			err := exec.Command(binary, args...).Run()
			var exitError *exec.ExitError
			if !errors.As(err, &exitError) {
				t.Fatalf("want non-zero exit, got %v", err)
			}
		})
	}
}

// The server must come up from -f alone (no kubeconfig, no DEMO_MODE) and
// serve the scenario cluster on every read endpoint the UI boots with.
func TestE2EServerStartsFromManifestDir(t *testing.T) {
	binary := buildBinary(t)

	cases := []struct {
		scenario   string
		namespaces []string
		nodeTypes  []models.NodeType
		engines    []string
	}{
		{
			scenario:   "01-k8s-baseline",
			namespaces: []string{"front", "back"},
			nodeTypes:  []models.NodeType{models.NodeTypeDeployment},
		},
		{
			scenario:   "02-istio-l7",
			namespaces: []string{"svc", "clients", "legacy", "istio-system"},
			engines:    []string{"istio"},
		},
		{
			// One node of every kind; the Succeeded pod must stay out.
			scenario:   "05-workload-kinds",
			namespaces: []string{"kinds"},
			nodeTypes: []models.NodeType{
				models.NodeTypeDeployment, models.NodeTypeStatefullSet, models.NodeTypeDaemonset,
				models.NodeTypeCronJob, models.NodeTypeJob, models.NodeTypePod,
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.scenario, func(t *testing.T) {
			baseURL := startServer(t, binary, filepath.Join(scenarioRoot, testCase.scenario))

			var state clusterStateDocument
			getJSON(t, baseURL+"/api/cluster-state", &state)
			for _, namespace := range testCase.namespaces {
				if !contains(state.AvailableNs, namespace) {
					t.Errorf("namespace %q missing from cluster-state: %v", namespace, state.AvailableNs)
				}
			}
			if len(state.StatusKeys) == 0 || len(state.PolicySources) == 0 {
				t.Errorf("cluster-state incomplete: %+v", state)
			}
			for _, engine := range testCase.engines {
				if !contains(state.PolicySources, engine) {
					t.Errorf("engine %q missing: %v", engine, state.PolicySources)
				}
			}

			var built graphDocument
			getJSON(t, baseURL+"/api/graph", &built)
			if len(built.Nodes) == 0 || len(built.Edges) == 0 {
				t.Fatalf("empty graph: %d nodes, %d edges", len(built.Nodes), len(built.Edges))
			}
			seenTypes := map[models.NodeType]bool{}
			for _, node := range built.Nodes {
				seenTypes[node.Type] = true
				if strings.Contains(node.Label, "finished") {
					t.Errorf("terminated pod rendered as a node: %+v", node)
				}
			}
			for _, nodeType := range testCase.nodeTypes {
				if !seenTypes[nodeType] {
					t.Errorf("node type %q missing from graph; saw %v", nodeType, seenTypes)
				}
			}

			// Remaining boot endpoints must answer 200 with a JSON body.
			for _, path := range []string{"/api/issues", "/api/mesh-status", "/api/cluster-metrics"} {
				var body json.RawMessage
				getJSON(t, baseURL+path, &body)
			}
		})
	}
}

// startServer launches the binary against a manifest dir on a free port and
// waits for it to answer. Kills it on test cleanup.
func startServer(t *testing.T, binary, manifestDir string) string {
	t.Helper()
	port := freePort(t)
	command := exec.Command(binary, "-f", manifestDir)
	command.Env = append(os.Environ(), "BACKEND_PORT="+port)
	var output strings.Builder
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Start(); err != nil {
		t.Fatalf("start %s: %v", manifestDir, err)
	}
	t.Cleanup(func() {
		_ = command.Process.Kill()
		_ = command.Wait()
	})

	baseURL := "http://127.0.0.1:" + port
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(baseURL + "/api/cluster-state")
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return baseURL
			}
		}
		if command.ProcessState != nil && command.ProcessState.Exited() {
			t.Fatalf("server exited early:\n%s", output.String())
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("server never became ready on %s:\n%s", baseURL, output.String())
	return ""
}

func getJSON(t *testing.T, url string, target any) {
	t.Helper()
	response, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d", url, response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatalf("GET %s: decode: %v", url, err)
	}
}

func freePort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
