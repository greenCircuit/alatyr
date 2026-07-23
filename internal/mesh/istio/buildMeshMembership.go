package istio

import (
	"context"
	"log/slog"

	"graph/internal/models"
)

// BuildMeshMembership resolves MeshMembership for every workload in the given
// ns index and tallies mesh MeshMetrics in the same pass. Enrollment is
// per-workload: workload label wins over ns label, root/ingress system
// workloads always participate (participatesInMesh). PA precedence walk:
// root PA fetched once, ns PA once per ns. NsTotal/WorkloadsTotal
// denominators are stamped by the store from cache sizes, not here.
func (s *source) BuildMeshMembership(nsIndex map[string]models.NSIndex) (models.MeshBuildResult, error) {
	var meshMetrics models.MeshMetrics
	var meshIssues []models.Issue
	membership := make(map[string]models.MeshMembership)

	// Degrade instead of fail: root PA fetch error means the precedence walk
	// misses the mesh-wide fallback, but every workload still gets an
	// InMesh verdict from its ns/workload PA. UI surfaces the fetch failure
	// via meshIssues; caller keeps the graph.
	globalPa, globalPaErr := s.client.GetPeerAuthentications(RootNamespace)
	if globalPaErr != nil {
		s.log.LogAttrs(context.TODO(), slog.LevelError, "mesh root PA fetch failed",
			slog.String("phase", "mesh_build_membership"),
			slog.String("ns", RootNamespace),
			slog.String("error", globalPaErr.Error()),
		)
		meshIssues = append(meshIssues, models.Issue{
			Type:    models.IssuesFailedToFetch,
			Message: "root peer authentications fetch failed for " + RootNamespace + ": " + globalPaErr.Error(),
			Engine:  SourceName,
		})
		globalPa = nil
	}

	for ns, idx := range nsIndex {
		// NSNode is nil when the ns object fetch raced a deletion — resolve
		// enrollment from workload labels alone instead of panicking.
		var nsLabels map[string]string
		if idx.NSNode != nil {
			nsLabels = idx.NSNode.Labels
		}

		var nsEnrolled bool
		var nsPartialRecorded bool
		if nsLabels[AmbientEnrollmentKey] == AmbientEnrollmentValue || ns == RootNamespace || ns == IngressNamespace {
			meshMetrics.NsEnrolled++
			nsEnrolled = true
		}

		// Degrade instead of fail: one flaky ns PA fetch must not tear down
		// /api/graph for the whole cluster. Enrolled workloads in this ns
		// still get InMesh=true from labels with an explicit unknown mTLS
		// verdict. UI surfaces the fetch failure via meshIssues.
		nsPa, paFetchError := s.client.GetPeerAuthentications(ns)
		if paFetchError != nil {
			s.log.LogAttrs(context.TODO(), slog.LevelError, "mesh ns PA fetch failed",
				slog.String("phase", "mesh_build_membership"),
				slog.String("ns", ns),
				slog.String("error", paFetchError.Error()),
			)
			meshIssues = append(meshIssues, models.Issue{
				Type:    models.IssuesFailedToFetch,
				Message: "peer authentications fetch failed for " + ns + ": " + paFetchError.Error(),
				Engine:  SourceName,
				Node:    idx.NSNode,
			})
		}

		for _, node := range idx.Workloads {
			if node.Type == models.NodeTypeNamespace {
				// Stamp the ns node with the ns-level verdict so the
				// mesh-membership filter can select enrolled namespaces (incl.
				// system namespaces — nsEnrolled already covers root/ingress).
				// Kept on its own branch so it never touches WorkloadsEnrolled or
				// the mTLS counters — NsEnrolled is already tallied once per ns.
				nsMembership := models.MeshMembership{InMesh: nsEnrolled}
				if nsEnrolled {
					nsMembership.Provider = SourceName
					nsMembership.Mode = "ambient"
					// Namespace-level mTLS = the default every unselected
					// workload in the ns inherits. Resolve the precedence walk
					// with a label-less workload so only ns-scoped + mesh PAs
					// apply (no workload PA can match empty labels). Fetch
					// failure → explicit unknown, never invented.
					if paFetchError != nil || globalPaErr != nil {
						nsMembership.Mtls = &models.MtlsState{Verdict: models.MeshUnknown}
					} else {
						nsMembership.Mtls = resolveMtls(models.WorkloadNode{Namespace: ns}, nsPa, globalPa)
					}
				}
				membership[node.ID] = nsMembership
				continue
			}

			// Workload label wins over ns label: explicit skip opts out of an
			// enrolled ns, explicit ambient opts into a non-enrolled ns. Istio
			// components (gateway/istiod/ztunnel/cni) carry dataplane-mode:none
			// because they are the mesh proxies, not ambient members — the
			// opt-out must not exclude them.
			optedOut := !isIstioComponent(node) &&
				node.Labels[AmbientEnrollmentKey] == AmbientSkipValue
			enrolled := !optedOut &&
				(nsEnrolled || node.Labels[AmbientEnrollmentKey] == AmbientEnrollmentValue)
			if !enrolled {
				if optedOut && nsEnrolled && !nsPartialRecorded {
					meshMetrics.NsPartial++
					nsPartialRecorded = true
				}
				membership[node.ID] = models.MeshMembership{InMesh: false}
				continue
			}
			meshMetrics.WorkloadsEnrolled++

			// Membership is known from labels even when a PA fetch failed;
			// the verdict is not — explicit unknown, never invented. Root
			// failure blanks all verdicts (shared RBAC — ns fetches fail with
			// it in practice); nil Mtls stays reserved for "not populated".
			if paFetchError != nil || globalPaErr != nil {
				meshMetrics.MtlsUnknown++
				membership[node.ID] = models.MeshMembership{
					InMesh:   true,
					Provider: SourceName,
					Mode:     "ambient",
					Mtls:     &models.MtlsState{Verdict: models.MeshUnknown},
				}
				continue
			}

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
				// Issue rows. Node pointer copied so appending the loop var
				// address doesn't smear across iterations.
				if len(mtls.Issues) > 0 {
					workloadCopy := node
					for _, hygieneIssue := range mtls.Issues {
						meshIssues = append(meshIssues, models.Issue{
							Type:     models.MeshMisconfig,
							Message:  hygieneIssue.Message,
							Engine:   SourceName,
							Node:     &workloadCopy,
							Culprits: hygieneIssue.Refs,
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
	}

	return models.MeshBuildResult{
		Memberships: membership,
		Metrics:     meshMetrics,
		Issues:      meshIssues,
	}, nil
}