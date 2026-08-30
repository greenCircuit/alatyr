package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"graph/internal/models"
)

func workload(id, namespace, label string, nodeType models.NodeType) models.WorkloadNode {
	return models.WorkloadNode{ID: id, Namespace: namespace, Label: label, Type: nodeType}
}

// countCoverage must count real workloads only, and must split "covered by any
// engine" from "covered by something other than the cluster-scoped engine".
func TestCountCoverage(t *testing.T) {
	nodes := []models.WorkloadNode{
		workload("front/gateway", "front", "gateway", models.NodeTypeDeployment),
		workload("front/scratch", "front", "scratch", models.NodeTypeDeployment),
		workload("back/db", "back", "db", models.NodeTypeStatefullSet),
		workload("front", "front", "front", models.NodeTypeNamespace),
		workload("cidr/10.0.0.0-8", "", "10.0.0.0/8", models.NodeTypeCIDR),
		workload("external", "", "internet", models.NodeTypeExternal),
	}
	cache := &models.Cache{EvaluationResults: map[string]models.EvaluationResult{
		"k8s": {NodePolicies: map[string][]models.PolicyRef{
			"front/gateway": {{Name: "allow-in", Namespace: "front"}},
		}},
		globalEngine: {NodePolicies: map[string][]models.PolicyRef{
			"front/gateway": {{Name: "gnp-default"}},
			"back/db":       {{Name: "gnp-default"}},
		}},
	}}

	out := report{}
	counts := countCoverage(cache, nodes, 7, &out)

	if counts.Workloads != 3 {
		t.Fatalf("workloads = %d, want 3 (synthetic nodes must be skipped)", counts.Workloads)
	}
	if counts.Manifests != 7 {
		t.Errorf("manifests = %d, want 7", counts.Manifests)
	}
	// db is covered by the global engine only, scratch by nothing.
	if counts.CoverageWithGlobal != 2 || counts.NoPolicyWithGlobals != 1 {
		t.Errorf("with globals: covered=%d uncovered=%d, want 2/1", counts.CoverageWithGlobal, counts.NoPolicyWithGlobals)
	}
	if counts.CoverageWithoutGlobal != 1 || counts.NoPolicyWithoutGlobals != 2 {
		t.Errorf("without globals: covered=%d uncovered=%d, want 1/2", counts.CoverageWithoutGlobal, counts.NoPolicyWithoutGlobals)
	}

	if len(out.NoCoverageWithGlobals) != 1 || out.NoCoverageWithGlobals[0].Node.ID != "front/scratch" {
		t.Errorf("uncovered-with-globals list = %+v, want only front/scratch", out.NoCoverageWithGlobals)
	}
	gotWithout := []string{}
	for _, missing := range out.NoCoverageWithoutGlobals {
		gotWithout = append(gotWithout, missing.Node.ID)
	}
	if strings.Join(gotWithout, ",") != "front/scratch,back/db" {
		t.Errorf("uncovered-without-globals = %v, want [front/scratch back/db]", gotWithout)
	}
}

// An engine entry with an empty policy list is not coverage — every engine
// emits an entry for every workload.
func TestCountCoverageEmptyPolicyListIsNotCoverage(t *testing.T) {
	nodes := []models.WorkloadNode{workload("front/gateway", "front", "gateway", models.NodeTypeDeployment)}
	cache := &models.Cache{EvaluationResults: map[string]models.EvaluationResult{
		"k8s": {NodePolicies: map[string][]models.PolicyRef{"front/gateway": {}}},
	}}

	out := report{}
	counts := countCoverage(cache, nodes, 0, &out)
	if counts.CoverageWithGlobal != 0 || counts.NoPolicyWithGlobals != 1 {
		t.Fatalf("covered=%d uncovered=%d, want 0/1", counts.CoverageWithGlobal, counts.NoPolicyWithGlobals)
	}
}

// countIssues seeds every known type/severity at zero and separates breaking
// from informational findings.
func TestCountIssues(t *testing.T) {
	issues := []models.Issue{
		{Type: models.NodeLockOut, Severity: models.IssueSeverityHigh},
		{Type: models.NodeLockOut, Severity: models.IssueSeverityHigh},
		{Type: models.NoDNSEgress, Severity: models.IssueSeverityWarning},
		{Type: models.IssuesPartial, Severity: models.IssueSeverityInfo},
		{Type: models.MeshMisconfig, Severity: models.IssueSeveritySecure},
	}

	counts := countIssues(issues)

	if counts.TotalIssues != 5 {
		t.Errorf("total = %d, want 5", counts.TotalIssues)
	}
	if counts.TotalIssuesBreaking != 3 {
		t.Errorf("breaking = %d, want 3 (info + secure excluded)", counts.TotalIssuesBreaking)
	}
	if counts.ByType[models.NodeLockOut] != 2 || counts.ByType[models.NoDNSEgress] != 1 {
		t.Errorf("byType = %v", counts.ByType)
	}
	if counts.BySeverity[models.IssueSeverityHigh] != 2 || counts.BySeverity[models.IssueSeverityInfo] != 1 {
		t.Errorf("bySeverity = %v", counts.BySeverity)
	}
	for issueType := range models.SeverityByType {
		if _, ok := counts.ByType[issueType]; !ok {
			t.Errorf("type %q missing from byType — consumers can't tell zero from gone", issueType)
		}
	}
	for _, severity := range []models.IssueSeverity{
		models.IssueSeverityCritical, models.IssueSeverityHigh, models.IssueSeverityWarning,
		models.IssueSeverityCaution, models.IssueSeverityInfo, models.IssueSeveritySecure,
	} {
		if _, ok := counts.BySeverity[severity]; !ok {
			t.Errorf("severity %q missing from bySeverity", severity)
		}
	}

	empty := countIssues(nil)
	if empty.TotalIssues != 0 || len(empty.ByType) != len(models.SeverityByType) {
		t.Errorf("empty run: total=%d types=%d", empty.TotalIssues, len(empty.ByType))
	}
}

func TestIsBreaking(t *testing.T) {
	breaking := []models.IssueSeverity{
		models.IssueSeverityCritical, models.IssueSeverityHigh,
		models.IssueSeverityWarning, models.IssueSeverityCaution,
	}
	for _, severity := range breaking {
		if !isBreaking(severity) {
			t.Errorf("%q must be breaking", severity)
		}
	}
	for _, severity := range []models.IssueSeverity{models.IssueSeverityInfo, models.IssueSeveritySecure} {
		if isBreaking(severity) {
			t.Errorf("%q must not be breaking", severity)
		}
	}
}

func TestCountManifests(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		filepath.Join(dir, "a.yaml"):       "kind: Namespace\n",
		filepath.Join(dir, "b.YML"):        "kind: Namespace\n",
		filepath.Join(dir, "README.md"):    "not a manifest\n",
		filepath.Join(nested, "c.yaml"):    "kind: Namespace\n",
		filepath.Join(nested, "notes.txt"): "nope\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	count, err := countManifests(dir)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("count = %d, want 3 (recursive, case-insensitive .yaml/.yml)", count)
	}

	if _, err := countManifests(filepath.Join(dir, "missing")); err == nil {
		t.Error("missing dir must error, not report zero manifests")
	}
}

func TestIssueScope(t *testing.T) {
	source := workload("front/gateway", "front", "gateway", models.NodeTypeDeployment)
	destination := workload("back/api", "back", "api", models.NodeTypeDeployment)
	clusterScoped := workload("cidr/10.0.0.0-8", "", "10.0.0.0/8", models.NodeTypeCIDR)

	cases := map[string]struct {
		issue models.Issue
		want  string
	}{
		"edge scoped":    {models.Issue{Src: &source, Dst: &destination}, "front/gateway → back/api"},
		"node scoped":    {models.Issue{Node: &destination}, "back/api"},
		"no namespace":   {models.Issue{Node: &clusterScoped}, "10.0.0.0/8"},
		"cluster scoped": {models.Issue{}, "cluster"},
	}
	for name, testCase := range cases {
		if got := issueScope(testCase.issue); got != testCase.want {
			t.Errorf("%s: scope = %q, want %q", name, got, testCase.want)
		}
	}
}

// culpritList merges all three culprit buckets, dedupes, and caps the row.
func TestCulpritList(t *testing.T) {
	if got := culpritList(models.Issue{}); got != "-" {
		t.Errorf("no culprits = %q, want -", got)
	}

	merged := culpritList(models.Issue{
		IngressCulprits: []models.PolicyRef{{Name: "deny-all", Namespace: "back"}},
		EgressCulprits:  []models.PolicyRef{{Name: "deny-all", Namespace: "back"}, {Name: "gnp-default"}},
		Culprits:        []models.PolicyRef{{Name: "peer-auth", Namespace: "back"}},
	})
	if merged != "back/deny-all, gnp-default, back/peer-auth" {
		t.Errorf("merged = %q", merged)
	}

	var many []models.PolicyRef
	for _, name := range []string{"one", "two", "three", "four", "five"} {
		many = append(many, models.PolicyRef{Name: name, Namespace: "back"})
	}
	if got := culpritList(models.Issue{IngressCulprits: many}); got != "back/one, back/two, back/three (+2)" {
		t.Errorf("capped = %q", got)
	}
}

// renderTable is the triage surface: coverage block, severity summary, and
// actionable findings worst-first with info findings summarized only.
func TestRenderTable(t *testing.T) {
	lockedOut := workload("back/db", "back", "db", models.NodeTypeStatefullSet)
	source := workload("front/gateway", "front", "gateway", models.NodeTypeDeployment)

	out := report{
		Target:    "test-data/scenarios/01-k8s-baseline",
		MeshCount: models.MeshMetrics{NsEnrolled: 1},
		Counts:    reportCount{Workloads: 3, Manifests: 7, CoverageWithGlobal: 2, NoPolicyWithGlobals: 1},
		Issues: []models.Issue{
			{Type: models.IssuesPartial, Severity: models.IssueSeverityInfo, Node: &lockedOut},
			{Type: models.NoDNSEgress, Severity: models.IssueSeverityWarning, Node: &lockedOut, Engine: "k8s"},
			{
				Type: models.NodeLockOut, Severity: models.IssueSeverityHigh, Node: &source, Engine: "k8s",
				IngressCulprits: []models.PolicyRef{{Name: "deny-all", Namespace: "front"}},
			},
		},
	}
	out.IssuesCount = countIssues(out.Issues)

	var buffer strings.Builder
	if err := renderTable(&buffer, out); err != nil {
		t.Fatal(err)
	}
	rendered := buffer.String()

	for _, want := range []string{
		"test-data/scenarios/01-k8s-baseline",
		"3 workloads, 7 manifests, 1 namespaces in mesh",
		"including global policies",
		"excluding global policies",
		"front/deny-all",
		"3 findings (2 actionable, 1 informational)",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("output missing %q:\n%s", want, rendered)
		}
	}

	// Worst-first in the findings table, and the info finding stays out of it.
	highRow := strings.Index(rendered, string(models.NodeLockOut))
	warningRow := strings.Index(rendered, string(models.NoDNSEgress))
	if highRow < 0 || warningRow < 0 || highRow > warningRow {
		t.Errorf("findings not worst-first (high=%d warning=%d):\n%s", highRow, warningRow, rendered)
	}
	findingsSection := rendered[strings.LastIndex(rendered, "SEVERITY"):]
	if strings.Contains(findingsSection, string(models.IssuesPartial)) {
		t.Errorf("info finding leaked into the findings table:\n%s", findingsSection)
	}
	tableRows := strings.Split(strings.TrimSpace(strings.SplitN(findingsSection, "\n\n", 2)[0]), "\n")
	if len(tableRows) != 3 { // header + 2 actionable rows
		t.Errorf("findings table has %d lines, want header + 2 rows:\n%s", len(tableRows), findingsSection)
	}
}

func TestRunRejectsUnknownFormat(t *testing.T) {
	err := Run(Options{ManifestDir: filepath.Join("..", "test-data", "scenarios", "01-k8s-baseline"), Format: "yaml"})
	if err == nil || !strings.Contains(err.Error(), "unknown format") {
		t.Fatalf("err = %v, want unknown format", err)
	}
}

func TestRunFailsOnMissingDir(t *testing.T) {
	if err := Run(Options{ManifestDir: filepath.Join(t.TempDir(), "missing")}); err == nil {
		t.Fatal("missing manifest dir must error")
	}
}

// Run writes the same document to --output that --format json prints, and the
// JSON stays parseable by jq-style consumers.
func TestRunWritesJSONFile(t *testing.T) {
	jsonPath := filepath.Join(t.TempDir(), "report.json")
	options := Options{
		ManifestDir: filepath.Join("..", "test-data", "scenarios", "01-k8s-baseline"),
		Format:      "table",
		JSONPath:    jsonPath,
	}
	if err := Run(options); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	var decoded report
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("report file is not valid JSON: %v", err)
	}
	if decoded.Target != options.ManifestDir {
		t.Errorf("target = %q, want %q", decoded.Target, options.ManifestDir)
	}
	if decoded.Counts.Workloads == 0 || decoded.Counts.Manifests == 0 {
		t.Errorf("empty counts in written report: %+v", decoded.Counts)
	}
}
