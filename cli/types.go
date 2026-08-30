package cli

import "graph/internal/models"

// report is the machine-readable output of a scan. Every field is exported and
// tagged — end users parse this with jq, so shape stability matters more than
// convenience. Metrics first, per-finding detail last.
type report struct {
	Target string `json:"target"`

	Counts              reportCount        `json:"counts"`
	BreakingIssuesCount issuesCount        `json:"breakingIssuesCount"` // excludes info-severity issues
	IssuesCount         issuesCount        `json:"issuesCount"`
	MeshCount           models.MeshMetrics `json:"meshCount"`

	Issues                   []models.Issue    `json:"issues"`
	NoCoverageWithGlobals    []missingCoverage `json:"noCoverageWithGlobals,omitempty"`
	NoCoverageWithoutGlobals []missingCoverage `json:"noCoverageWithoutGlobals,omitempty"`
}

type reportCount struct {
	Workloads              int `json:"workloads"`
	Manifests              int `json:"manifests"`
	CoverageWithGlobal     int `json:"coverageWithGlobal"`
	CoverageWithoutGlobal  int `json:"coverageWithoutGlobal"`
	NoPolicyWithGlobals    int `json:"noPolicyWithGlobals"`
	NoPolicyWithoutGlobals int `json:"noPolicyWithoutGlobals"`
}

type issuesCount struct {
	TotalIssues int                          `json:"totalIssues"`
	TotalIssuesBreaking              int     `json:"totalIssuesBreaking"`  // not Info issues
	ByType      map[models.IssueType]int     `json:"byType"`
	BySeverity  map[models.IssueSeverity]int `json:"bySeverity"`
}

type missingCoverage struct {
	Node models.WorkloadNode `json:"node"`
}
