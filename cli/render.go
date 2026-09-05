package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"alatyr/internal/models"
)

// severityOrder ranks findings worst-first in the table. Table output is a
// triage surface — an operator reads the top rows and stops.
var severityOrder = map[models.IssueSeverity]int{
	models.IssueSeverityCritical: 0,
	models.IssueSeverityHigh:     1,
	models.IssueSeverityWarning:  2,
	models.IssueSeverityCaution:  3,
	models.IssueSeverityInfo:     4,
	models.IssueSeveritySecure:   5,
}

// renderTable writes the human-readable report: totals, a severity line, a
// per-type breakdown, then one row per breaking finding. Info findings are
// summarized only — they are expected layering, not work.
func renderTable(writer io.Writer, out report) error {
	_, _ = fmt.Fprintf(writer, "\nmanifests scan %s\n", out.Target)
	_, _ = fmt.Fprintf(writer, "%d workloads, %d manifests, %d namespaces in mesh\n\n",
		out.Counts.Workloads, out.Counts.Manifests, out.MeshCount.NsEnrolled)

	renderCoverage(writer, out.Counts)
	renderInternetExposure(writer, out.InternetExposure)
	renderSeveritySummary(writer, out.IssuesCount)
	renderFindings(writer, out.Issues)

	_, _ = fmt.Fprintf(writer, "\n%d findings (%d actionable, %d informational)\n",
		out.IssuesCount.TotalIssues,
		out.IssuesCount.TotalIssuesBreaking,
		out.IssuesCount.TotalIssues-out.IssuesCount.TotalIssuesBreaking)
	return nil
}

func renderCoverage(writer io.Writer, counts reportCount) {
	table := tabwriter.NewWriter(writer, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(table, "COVERAGE\tCOVERED\tUNCOVERED")
	_, _ = fmt.Fprintf(table, "including global policies\t%d\t%d\n", counts.CoverageWithGlobal, counts.NoPolicyWithGlobals)
	_, _ = fmt.Fprintf(table, "excluding global policies\t%d\t%d\n", counts.CoverageWithoutGlobal, counts.NoPolicyWithoutGlobals)
	_ = table.Flush()
	_, _ = fmt.Fprintln(writer)
}

// renderInternetExposure prints the wan blast radius: how many workloads are
// open to the internet and in which direction.
func renderInternetExposure(writer io.Writer, exposure exposureCount) {
	table := tabwriter.NewWriter(writer, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(table, "INTERNET EXPOSURE\tWORKLOADS")
	_, _ = fmt.Fprintf(table, "both directions (internet-full)\t%d\n", exposure.Full)
	_, _ = fmt.Fprintf(table, "inbound only (internet-ingress)\t%d\n", exposure.Ingress)
	_, _ = fmt.Fprintf(table, "outbound only (internet-egress)\t%d\n", exposure.Egress)
	_, _ = fmt.Fprintf(table, "total exposed\t%d\n", exposure.Total)
	_ = table.Flush()
	_, _ = fmt.Fprintln(writer)
}

func renderSeveritySummary(writer io.Writer, counts issuesCount) {
	var parts []string
	for _, severity := range []models.IssueSeverity{
		models.IssueSeverityCritical, models.IssueSeverityHigh,
		models.IssueSeverityWarning, models.IssueSeverityCaution, models.IssueSeverityInfo,
	} {
		parts = append(parts, fmt.Sprintf("%s: %d", severity, counts.BySeverity[severity]))
	}
	_, _ = fmt.Fprintf(writer, "Findings — %s\n\n", strings.Join(parts, "  "))

	types := make([]models.IssueType, 0, len(counts.ByType))
	for issueType, count := range counts.ByType {
		if count > 0 {
			types = append(types, issueType)
		}
	}
	sort.Slice(types, func(left, right int) bool {
		leftRank := severityOrder[models.SeverityForType(types[left])]
		rightRank := severityOrder[models.SeverityForType(types[right])]
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return types[left] < types[right]
	})

	table := tabwriter.NewWriter(writer, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(table, "TYPE\tSEVERITY\tCOUNT")
	for _, issueType := range types {
		_, _ = fmt.Fprintf(table, "%s\t%s\t%d\n", issueType, models.SeverityForType(issueType), counts.ByType[issueType])
	}
	_ = table.Flush()
	_, _ = fmt.Fprintln(writer)
}

// renderFindings lists actionable findings only, worst-first, one line each.
func renderFindings(writer io.Writer, issues []models.Issue) {
	breaking := make([]models.Issue, 0, len(issues))
	for _, issue := range issues {
		if isBreaking(issue.Severity) {
			breaking = append(breaking, issue)
		}
	}
	if len(breaking) == 0 {
		return
	}

	sort.SliceStable(breaking, func(left, right int) bool {
		leftRank := severityOrder[breaking[left].Severity]
		rightRank := severityOrder[breaking[right].Severity]
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if breaking[left].Type != breaking[right].Type {
			return breaking[left].Type < breaking[right].Type
		}
		return issueScope(breaking[left]) < issueScope(breaking[right])
	})

	table := tabwriter.NewWriter(writer, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(table, "SEVERITY\tTYPE\tSCOPE\tENGINE\tCULPRITS")
	for _, issue := range breaking {
		_, _ = fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n",
			issue.Severity, issue.Type, issueScope(issue), issue.Engine, culpritList(issue))
	}
	_ = table.Flush()
}

// issueScope renders the endpoint an operator has to go fix: src→dst for
// edge-scoped findings, the node itself otherwise.
func issueScope(issue models.Issue) string {
	if issue.Src != nil && issue.Dst != nil {
		return fmt.Sprintf("%s → %s", nodeScope(issue.Src), nodeScope(issue.Dst))
	}
	if issue.Node != nil {
		return nodeScope(issue.Node)
	}
	return "cluster"
}

func nodeScope(node *models.WorkloadNode) string {
	if node.Namespace == "" {
		return node.Label
	}
	return node.Namespace + "/" + node.Label
}

// culpritList names the policies to edit, deduped, capped so one row stays one
// line. Ingress and egress culprits both point at real work.
func culpritList(issue models.Issue) string {
	const maxNames = 3
	seen := map[string]bool{}
	var names []string
	for _, group := range [][]models.PolicyRef{issue.IngressCulprits, issue.EgressCulprits, issue.Culprits} {
		for _, ref := range group {
			name := ref.Name
			if ref.Namespace != "" {
				name = ref.Namespace + "/" + ref.Name
			}
			if seen[name] {
				continue
			}
			seen[name] = true
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return "-"
	}
	if len(names) > maxNames {
		return fmt.Sprintf("%s (+%d)", strings.Join(names[:maxNames], ", "), len(names)-maxNames)
	}
	return strings.Join(names, ", ")
}
