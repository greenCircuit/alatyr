package store

import (
	"testing"

	"graph/internal/models"
)

// BuildNodeNeighbor feeds the click-on-node detail panel: per-engine In/Out
// policy neighbors. The bug it fixes: a node's cross-namespace rules land in the
// POLICY's ns bucket (ingress rules flip endpoints), not the node's own ns, so
// scanning one bucket dropped them. These tests pin the all-buckets walk plus
// the no-dedup / deny / self-loop / unresolved-peer edge cases. Reuses the
// srcNs/srcID/dstNs/dstID consts from buildStore_test.go.

const cidrPeer = "10.0.0.0/8"

func TestBuildNodeNeighbor_CrossNsBucketsInboundOutbound(t *testing.T) {
	// Every rule touching srcID is bucketed under dstNs — the POLICY's namespace,
	// NOT srcID's own ns. A per-ns scan would return nothing; the all-buckets
	// walk must find them. This is the exact cross-ns blind spot.
	allow := map[string][]models.Rule{
		dstNs: {
			{SrcID: srcID, DstID: dstID, Direction: models.DirectionEgress, Ports: []models.Port{{Port: 80, Protocol: "TCP"}}},  // Out, resolved
			{SrcID: srcID, DstID: dstID, Direction: models.DirectionEgress, Ports: []models.Port{{Port: 443, Protocol: "TCP"}}}, // Out, SAME pair — no dedup
			{SrcID: dstID, DstID: srcID, Direction: models.DirectionIngress},                                                     // In, resolved peer
			{SrcID: srcID, DstID: srcID, Direction: models.DirectionEgress},                                                      // self-loop — skipped
			{SrcID: srcID, DstID: cidrPeer, Direction: models.DirectionEgress},                                                   // Out, unresolved external
		},
	}
	deny := map[string][]models.Rule{
		dstNs: {
			{SrcID: srcID, DstID: dstID, Direction: models.DirectionEgress, Action: models.ActionDeny}, // Out, deny — DenyByNs must be walked
		},
	}

	cache := &models.Cache{
		NsIndex: map[string]models.NSIndex{
			srcNs: {Workloads: []models.WorkloadNode{{ID: srcID, Label: "src-pod", Namespace: srcNs}}},
			dstNs: {Workloads: []models.WorkloadNode{{ID: dstID, Label: "dst-pod", Namespace: dstNs}}},
		},
		EvaluationResults: map[string]models.EvaluationResult{
			"k8s": {AllowByNs: allow, DenyByNs: deny},
		},
	}
	cache.RebuildWorkloadIndex()

	got := BuildNodeNeighbor(cache, srcID)

	neighbors, ok := got["k8s"]
	if !ok {
		t.Fatalf("k8s engine missing: %v", got)
	}

	// Out = 2 same-pair allow + 1 external + 1 deny = 4 (self-loop dropped, no dedup).
	if len(neighbors.Out) != 4 {
		t.Fatalf("want 4 outbound neighbors, got %d: %+v", len(neighbors.Out), neighbors.Out)
	}
	// In = the single flipped ingress rule.
	if len(neighbors.In) != 1 {
		t.Fatalf("want 1 inbound neighbor, got %d: %+v", len(neighbors.In), neighbors.In)
	}

	var sawDeny, sawResolved, sawExternal bool
	for _, neighbor := range neighbors.Out {
		if neighbor.Rule.SrcID == neighbor.Rule.DstID {
			t.Errorf("self-loop leaked into Out: %+v", neighbor.Rule)
		}
		switch neighbor.Rule.DstID {
		case dstID:
			if neighbor.Rule.Action == models.ActionDeny {
				sawDeny = true
			}
			if neighbor.Workload.Label == "dst-pod" && neighbor.Workload.Namespace == dstNs {
				sawResolved = true // cross-ns peer resolved via the flat id index
			}
		case cidrPeer:
			sawExternal = true
			if neighbor.Workload.ID != "" {
				t.Errorf("external CIDR peer should have blank workload, got %+v", neighbor.Workload)
			}
		}
	}
	if !sawDeny {
		t.Error("deny rule from DenyByNs missing — DenyByNs not walked")
	}
	if !sawResolved {
		t.Error("cross-ns peer not resolved to its workload")
	}
	if !sawExternal {
		t.Error("unresolved external CIDR neighbor was dropped — must be kept with blank workload")
	}

	// Inbound peer (srcID is the destination) resolves to dstID's workload.
	if neighbors.In[0].Workload.Label != "dst-pod" {
		t.Errorf("inbound peer not resolved: %+v", neighbors.In[0].Workload)
	}
}

func TestBuildNodeNeighbor_PerEngineSeparation(t *testing.T) {
	cache := &models.Cache{
		NsIndex: map[string]models.NSIndex{
			dstNs: {Workloads: []models.WorkloadNode{{ID: dstID, Label: "dst-pod", Namespace: dstNs}}},
		},
		EvaluationResults: map[string]models.EvaluationResult{
			"k8s":   {AllowByNs: map[string][]models.Rule{srcNs: {{SrcID: srcID, DstID: dstID}}}},
			"istio": {DenyByNs: map[string][]models.Rule{dstNs: {{SrcID: srcID, DstID: dstID, Action: models.ActionDeny}}}},
		},
	}
	cache.RebuildWorkloadIndex()

	got := BuildNodeNeighbor(cache, srcID)

	if len(got["k8s"].Out) != 1 || got["k8s"].Out[0].Rule.Action != models.ActionAllow {
		t.Errorf("k8s engine: want 1 allow neighbor, got %+v", got["k8s"].Out)
	}
	if len(got["istio"].Out) != 1 || got["istio"].Out[0].Rule.Action != models.ActionDeny {
		t.Errorf("istio engine: want 1 deny neighbor, got %+v", got["istio"].Out)
	}
}
