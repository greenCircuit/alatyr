package cli

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"graph/internal/graph"
	"graph/internal/k8s"
	"graph/internal/logging"
	"graph/internal/models"
	"graph/internal/store"
)

// globalEngine is the cluster-scoped engine. Coverage is reported twice — with
// and without it — so an operator can tell real per-workload policy from a
// blanket global rule that happens to select everything.
const globalEngine = "calico"

func makeReport(manifestDir string, logger *slog.Logger) (report, error) {
	if logger == nil {
		logger = slog.Default()
	}

	var client k8s.KubernetesClient
	demo, err := k8s.NewDemoClient(os.DirFS(manifestDir), ".")
	if err != nil {
		return report{}, fmt.Errorf("failed to parse input dir %s: %w", manifestDir, err)
	}
	client = demo

	cache := &models.Cache{}
	namespaces, err := client.GetNsNames()
	if err != nil {
		return report{}, fmt.Errorf("failed to populate ns: %w", err)
	}

	builder := store.NewBuilder(client, logger)
	if err := builder.PopulateCache(cache, namespaces); err != nil {
		return report{}, fmt.Errorf("failed to populate gathered manifests: %w", err)
	}

	manifests, err := countManifests(manifestDir)
	if err != nil {
		return report{}, err
	}

	ctx := logging.WithLogger(context.Background(), logger)
	built := graph.BuildGraph(cache, namespaces)

	out := report{
		Target:    manifestDir,
		MeshCount: cache.MeshMetrics,
		Issues:    store.GetIssues(ctx, cache, builder.MeshSource()),
	}
	out.Counts = countCoverage(cache, built.Nodes, manifests, &out)
	out.IssuesCount = countIssues(out.Issues)

	// Same tally over the actionable subset — CI gates read this one, so info
	// findings (expected layering) must not inflate it.
	var breaking []models.Issue
	for _, issue := range out.Issues {
		if isBreaking(issue.Severity) {
			breaking = append(breaking, issue)
		}
	}
	out.BreakingIssuesCount = countIssues(breaking)
	return out, nil
}

// isBreaking marks a severity as actionable. Info = expected layering, secure
// = a good posture finding; neither should fail a pipeline.
func isBreaking(severity models.IssueSeverity) bool {
	return severity != models.IssueSeverityInfo && severity != models.IssueSeveritySecure
}

// countCoverage tallies workload totals and collects uncovered nodes onto out.
// Synthetic nodes (namespace, CIDR, external) are skipped — counting them
// inflates the denominator and makes coverage read better than it is.
func countCoverage(cache *models.Cache, nodes []models.WorkloadNode, manifests int, out *report) reportCount {
	counts := reportCount{Manifests: manifests}
	for _, node := range nodes {
		if node.Type == models.NodeTypeNamespace || node.Type == models.NodeTypeCIDR || node.Type == models.NodeTypeExternal {
			continue
		}
		counts.Workloads++

		// Coverage = a policy actually selects the workload. StatusesBySource
		// can't answer this: every engine emits an entry for every workload,
		// unconstrained ones included.
		covered := false
		coveredWithoutGlobal := false
		for engineName, evaluation := range cache.EvaluationResults {
			if len(evaluation.NodePolicies[node.ID]) == 0 {
				continue
			}
			covered = true
			if engineName != globalEngine {
				coveredWithoutGlobal = true
			}
		}

		if covered {
			counts.CoverageWithGlobal++
		} else {
			counts.NoPolicyWithGlobals++
			out.NoCoverageWithGlobals = append(out.NoCoverageWithGlobals, missingCoverage{Node: node})
		}

		if coveredWithoutGlobal {
			counts.CoverageWithoutGlobal++
		} else {
			counts.NoPolicyWithoutGlobals++
			out.NoCoverageWithoutGlobals = append(out.NoCoverageWithoutGlobals, missingCoverage{Node: node})
		}
	}
	return counts
}

// countIssues seeds every known type and severity at zero so a consumer can
// tell "nothing fired" from "detector no longer exists".
func countIssues(issues []models.Issue) issuesCount {
	counts := issuesCount{
		ByType:     map[models.IssueType]int{},
		BySeverity: map[models.IssueSeverity]int{},
	}
	for issueType := range models.SeverityByType {
		counts.ByType[issueType] = 0
	}
	for _, severity := range []models.IssueSeverity{
		models.IssueSeverityCritical, models.IssueSeverityHigh, models.IssueSeverityWarning,
		models.IssueSeverityCaution, models.IssueSeverityInfo, models.IssueSeveritySecure,
	} {
		counts.BySeverity[severity] = 0
	}

	for _, issue := range issues {
		counts.TotalIssues++
		counts.ByType[issue.Type]++
		counts.BySeverity[issue.Severity]++
		if isBreaking(issue.Severity) {
			counts.TotalIssuesBreaking++
		}
	}
	return counts
}

func countManifests(dir string) (int, error) {
	count := 0
	err := fs.WalkDir(os.DirFS(dir), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		extension := strings.ToLower(filepath.Ext(entry.Name()))
		if !entry.IsDir() && (extension == ".yaml" || extension == ".yml") {
			count++
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("walk %s: %w", dir, err)
	}
	return count, nil
}
