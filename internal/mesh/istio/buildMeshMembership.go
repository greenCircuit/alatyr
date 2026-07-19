package istio

import (
	"context"
	"log/slog"

	"graph/internal/models"
)

// BuildMeshMembership resolves MeshMembership for every workload in the given
// ns index and tallies cluster-wide MeshMetrics in the same pass. Ambient-
// enrolled namespaces get InMesh=true plus MtlsState from a single-shot PA
// precedence walk (root PA fetched once, ns PA once per ns). Non-enrolled
// namespaces get InMesh=false with no PA fetch.
func (s *source) BuildMeshMembership(nsIndex map[string]models.NSIndex) (models.MeshBuildResult, error) {
	var meshMetrics models.MeshMetrics
	var meshIssues []models.Issue
	membership := make(map[string]models.MeshMembership)

	// Degrade instead of fail: root PA fetch error means the precedence walk
	// misses the mesh-wide fallback, but every workload still gets an
	// InMesh verdict from its ns/workload PA. UI surfaces the fetch failure
	// via meshIssues; caller keeps the graph.
	globalPa, err := s.client.GetPeerAuthentications(RootNamespace)
	if err != nil {
		s.log.LogAttrs(context.TODO(), slog.LevelError, "mesh root PA fetch failed",
			slog.String("phase", "mesh_build_membership"),
			slog.String("ns", RootNamespace),
			slog.String("error", err.Error()),
		)
		meshIssues = append(meshIssues, models.Issue{
			Type:    models.MeshMisconfig,
			Message: "root peer authentications fetch failed for " + RootNamespace + ": " + err.Error(),
			Engine:  SourceName,
		})
		globalPa = nil
	}

	for ns, idx := range nsIndex {
		meshMetrics.NsTotal++
		nsLabels := idx.NSNode.Labels
		if nsLabels[AmbientEnrollmentKey] != AmbientEnrollmentValue {
			for _, node := range idx.Workloads {
				if node.Type == models.NodeTypeNamespace {
					continue
				}
				meshMetrics.WorkloadsTotal++
				membership[node.ID] = models.MeshMembership{InMesh: false}
			}
			continue
		}

		// Degrade instead of fail: one flaky ns PA fetch must not tear down
		// /api/graph for the whole cluster. Workloads in this ns still get
		// InMesh=true from the ns label, but Mtls stays nil (unknown). UI
		// surfaces the fetch failure via meshIssues.
		nsPa, err := s.client.GetPeerAuthentications(ns)
		if err != nil {
			s.log.LogAttrs(context.TODO(), slog.LevelError, "mesh ns PA fetch failed",
				slog.String("phase", "mesh_build_membership"),
				slog.String("ns", ns),
				slog.String("error", err.Error()),
			)
			meshIssues = append(meshIssues, models.Issue{
				Type:    models.MeshMisconfig,
				Message: "peer authentications fetch failed for " + ns + ": " + err.Error(),
				Engine:  SourceName,
				Node:    idx.NSNode,
			})
			for _, node := range idx.Workloads {
				if node.Type == models.NodeTypeNamespace {
					continue
				}
				meshMetrics.WorkloadsTotal++
				meshMetrics.WorkloadsEnrolled++
				membership[node.ID] = models.MeshMembership{
					InMesh:   true,
					Provider: SourceName,
					Mode:     "ambient",
					Mtls:     nil,
				}
			}
			meshMetrics.NsEnrolled++
			continue
		}
		meshMetrics.NsEnrolled++

		// Track per-ns enrollment split so we can flag partial enrollment
		// (workload label overrides ns label — some workloads may opt out).
		var enrolledInNs, optedOutInNs int
		for _, node := range idx.Workloads {
			if node.Type == models.NodeTypeNamespace {
				continue
			}
			meshMetrics.WorkloadsTotal++

			if !inAmbientMesh(node.Labels, nsLabels) {
				optedOutInNs++
				membership[node.ID] = models.MeshMembership{InMesh: false}
				continue
			}
			enrolledInNs++
			meshMetrics.WorkloadsEnrolled++

			mtls := resolveMtls(node, nsPa, globalPa)
			if mtls != nil {
				switch mtls.Verdict {
				case models.MeshStrict:
					meshMetrics.MtlsStrict++
				case models.MeshPermissive:
					meshMetrics.MtlsPermissive++
				case models.MeshDisable:
					meshMetrics.MtlsDisabled++
				case models.MeshUnset:
					meshMetrics.MtlsUnset++
				}
				// Promote per-workload PA hygiene warnings to cluster-wide
				// Issue rows so /api/issues surfaces them alongside policy
				// conflicts. Node pointer copied so appending the loop var
				// address doesn't smear across iterations.
				if len(mtls.Issues) > 0 {
					workloadCopy := node
					for _, msg := range mtls.Issues {
						meshIssues = append(meshIssues, models.Issue{
							Type:    models.MeshMisconfig,
							Message: msg,
							Engine:  SourceName,
							Node:    &workloadCopy,
						})
					}
				}
			}
			membership[node.ID] = models.MeshMembership{
				InMesh:   true,
				Provider: SourceName,
				Mode:     "ambient",
				Mtls:     mtls,
			}
		}
		if enrolledInNs > 0 && optedOutInNs > 0 {
			meshMetrics.NsPartial++
		}
	}

	return models.MeshBuildResult{
		Memberships: membership,
		Metrics:     meshMetrics,
		Issues:      meshIssues,
	}, nil
}
