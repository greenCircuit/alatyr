package graph

import (
	"testing"

	"graph/internal/models"
	"graph/internal/utils"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// BuildWorkloadIndex

func TestBuildWorkloadIndex_SingleLabel(t *testing.T) {
	nodes := []models.WorkloadNode{
		{ID: "a", Labels: map[string]string{"app": "web"}},
	}
	idx := BuildWorkloadIndex(nodes)
	bucket := idx[utils.MakeLabelIndexKey("app", "web")]
	if len(bucket) != 1 || bucket[0].ID != "a" {
		t.Errorf("expected node a in bucket, got %v", bucket)
	}
}

func TestBuildWorkloadIndex_MultiLabel(t *testing.T) {
	nodes := []models.WorkloadNode{
		{ID: "a", Labels: map[string]string{"app": "web", "env": "prod"}},
	}
	idx := BuildWorkloadIndex(nodes)
	if len(idx[utils.MakeLabelIndexKey("app", "web")]) != 1 {
		t.Error("expected node in app=web bucket")
	}
	if len(idx[utils.MakeLabelIndexKey("env", "prod")]) != 1 {
		t.Error("expected node in env=prod bucket")
	}
}

func TestBuildWorkloadIndex_SharedBucket(t *testing.T) {
	nodes := []models.WorkloadNode{
		{ID: "a", Labels: map[string]string{"app": "api"}},
		{ID: "b", Labels: map[string]string{"app": "api"}},
	}
	idx := BuildWorkloadIndex(nodes)
	bucket := idx[utils.MakeLabelIndexKey("app", "api")]
	if len(bucket) != 2 {
		t.Errorf("expected 2 nodes in shared bucket, got %d", len(bucket))
	}
}

func TestBuildWorkloadIndex_EmptyNodes(t *testing.T) {
	idx := BuildWorkloadIndex(nil)
	if len(idx) != 0 {
		t.Errorf("expected empty index, got %d entries", len(idx))
	}
}

func TestWorkloadLabel_MissingLabels(t *testing.T) {
	var pod corev1.Pod
	pod.Name = "test"
	res := WorkloadLabel(&pod)
	if res != pod.Name {
		t.Errorf("failed to get name when no owner reference and no labels")
	}
}

// AssembleNsIndex — pod ownership classification

func ownerRef(kind, name, uid string, controller bool) metav1.OwnerReference {
	return metav1.OwnerReference{Kind: kind, Name: name, UID: types.UID(uid), Controller: &controller}
}

func testPod(name, uid string, phase corev1.PodPhase, owners ...metav1.OwnerReference) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, UID: types.UID(uid), Namespace: "ns1", OwnerReferences: owners,
		},
		Status: corev1.PodStatus{Phase: phase},
	}
}

func testJob(name, uid string, owners ...metav1.OwnerReference) *batchv1.Job {
	return &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
		Name: name, UID: types.UID(uid), Namespace: "ns1", OwnerReferences: owners,
	}}
}

// Covers every pod-owner branch: bare, controller-with-own-node, bare Job,
// CronJob-owned Job, lister miss, unknown controller kind, phase filter.
func TestAssembleNsIndex_PodOwnership(t *testing.T) {
	resources := NsResources{
		Namespace: &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns1"}},
		Jobs: []*batchv1.Job{
			testJob("migrate", "job-bare"),
			testJob("backup-1", "job-cron", ownerRef("CronJob", "backup", "cj-1", true)),
		},
		Pods: []*corev1.Pod{
			testPod("bare", "pod-bare", corev1.PodRunning),
			testPod("crashloop", "pod-crash", corev1.PodRunning),
			testPod("web", "pod-web", corev1.PodRunning, ownerRef("ReplicaSet", "web-abc", "rs-1", true)),
			testPod("done", "pod-done", corev1.PodSucceeded),
			testPod("migrate-x", "pod-mig", corev1.PodRunning, ownerRef("Job", "migrate", "job-bare", true)),
			testPod("backup-x", "pod-bak", corev1.PodRunning, ownerRef("Job", "backup-1", "job-cron", true)),
			testPod("ghost-x", "pod-ghost", corev1.PodRunning, ownerRef("Job", "ghost", "job-missing", true)),
			testPod("kafka-0", "pod-kafka", corev1.PodRunning, ownerRef("Kafka", "events", "cr-1", true)),
			// non-controller ref first — controller ref must still win
			testPod("side", "pod-side", corev1.PodRunning,
				ownerRef("Secret", "tls", "sec-1", false),
				ownerRef("Job", "migrate2", "job-bare2", true)),
		},
	}
	resources.Jobs = append(resources.Jobs, testJob("migrate2", "job-bare2"))

	index := AssembleNsIndex(resources)

	got := map[string]models.NodeType{}
	for _, node := range index.Workloads {
		got[node.ID] = node.Type
	}

	want := map[string]models.NodeType{
		"pod-bare":    models.NodeTypePod,
		"pod-crash":   models.NodeTypePod,
		"job-bare":    models.NodeTypeJob,
		"job-missing": models.NodeTypeJob, // lister miss emits rather than drops
		"job-bare2":   models.NodeTypeJob,
		"cr-1":        models.NodeTypePod, // unknown controller kind still gets a node
		"ns-ns1":      models.NodeTypeNamespace,
	}
	if len(got) != len(want) {
		t.Fatalf("node set mismatch: got %v, want %v", got, want)
	}
	for id, nodeType := range want {
		if got[id] != nodeType {
			t.Errorf("node %s: got type %q, want %q", id, got[id], nodeType)
		}
	}

	for _, node := range index.Workloads {
		if node.ID == "job-bare" && node.Label != "migrate" {
			t.Errorf("bare Job node label: got %q, want %q", node.Label, "migrate")
		}
	}
}

func TestOwnerUID_IgnoresNonControllerRef(t *testing.T) {
	pod := testPod("side", "pod-1", corev1.PodRunning,
		ownerRef("Secret", "tls", "sec-1", false),
		ownerRef("ReplicaSet", "web-abc", "rs-1", true))
	if uid := OwnerUID(pod); uid != "rs-1" {
		t.Errorf("OwnerUID: got %q, want rs-1", uid)
	}
	if label := WorkloadLabel(pod); label != "web-abc" {
		t.Errorf("WorkloadLabel: got %q, want web-abc", label)
	}

	orphan := testPod("orphan", "pod-2", corev1.PodRunning, ownerRef("Secret", "tls", "sec-1", false))
	if uid := OwnerUID(orphan); uid != "pod-2" {
		t.Errorf("OwnerUID with no controller ref: got %q, want pod-2", uid)
	}
}
