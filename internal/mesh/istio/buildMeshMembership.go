package istio

import (
	"context"
	"log/slog"

	"graph/internal/models"
)

// BuildMeshMembership resolves MeshMembership for every workload in the given
// ns index. Ambient-enrolled namespaces get InMesh=true plus MtlsState from a
// single-shot PA precedence walk (root PA fetched once, ns PA once per ns).
// Non-enrolled namespaces get InMesh=false with no PA fetch.
func (s *source) BuildMeshMembership(nsIndex map[string]models.NSIndex) (map[string]models.MeshMembership, error) {
	membership := make(map[string]models.MeshMembership)

	globalPa, err := s.client.GetPeerAuthentications(RootNamespace)
	if err != nil {
		s.log.LogAttrs(context.TODO(), slog.LevelError, "mesh root PA fetch failed",
			slog.String("phase", "mesh_build_membership"),
			slog.String("ns", RootNamespace),
			slog.String("error", err.Error()),
		)
		return nil, err
	}

	for ns, idx := range nsIndex {
		nsLabels := idx.NSNode.Labels
		if nsLabels[AmbientEnrollmentKey] != AmbientEnrollmentValue {
			for _, node := range idx.Workloads {
				membership[node.ID] = models.MeshMembership{InMesh: false}
			}
			continue
		}

		nsPa, err := s.client.GetPeerAuthentications(ns)
		if err != nil {
			s.log.LogAttrs(context.TODO(), slog.LevelError, "mesh ns PA fetch failed",
				slog.String("phase", "mesh_build_membership"),
				slog.String("ns", ns),
				slog.String("error", err.Error()),
			)
			return nil, err
		}
		for _, node := range idx.Workloads {
			mtls := resolveMtls(node, nsPa, globalPa)
			membership[node.ID] = models.MeshMembership{
				InMesh:   true,
				Provider: SourceName,
				Mode:     "ambient",
				Mtls:     mtls,
			}
		}
	}

	return membership, nil
}
