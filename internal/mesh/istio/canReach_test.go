package istio

import (
	"log/slog"
	"testing"

	"graph/internal/models"
)

// mtls builds an MtlsState with just the verdict — enough for CanReach.
func mtls(scope models.MeshScope) *models.MtlsState {
	return &models.MtlsState{Verdict: scope}
}

// canReachSource returns a fresh source with a nil client — CanReach never
// touches the client after the signature change.
func canReachSource() *source {
	return &source{log: slog.Default()}
}

// dst not in mesh at all → allow. Guards the pre-mesh baseline: if the dst is
// plaintext-capable there is no STRICT gate to enforce.
func TestCanReach_DstNotInMesh(t *testing.T) {
	src := models.MeshMembership{InMesh: true, Mtls: mtls(models.MeshDisable)}
	dst := models.MeshMembership{InMesh: false}

	verdict := canReachSource().CanReach(src, dst, 0)

	if verdict.Verdict != "allow" {
		t.Fatalf("dst not in mesh: want allow, got %q (%s)", verdict.Verdict, verdict.Reason)
	}
}

// Dst is enrolled but Mtls is nil (BuildMeshMembership hasn't stamped it yet,
// or resolveMtls returned nil). Must NOT deref — allow instead. This is the
// exact footgun the new comment calls out.
func TestCanReach_DstMtlsNil(t *testing.T) {
	src := models.MeshMembership{InMesh: false}
	dst := models.MeshMembership{InMesh: true, Mtls: nil}

	verdict := canReachSource().CanReach(src, dst, 0)

	if verdict.Verdict != "allow" {
		t.Fatalf("nil dst Mtls: want allow (not deref), got %q (%s)", verdict.Verdict, verdict.Reason)
	}
}

// Dst enrolled, mTLS PERMISSIVE — permissive accepts plaintext, so a non-mesh
// src must not be denied. Regression guard: only STRICT triggers the gate.
func TestCanReach_DstPermissiveAllowsPlaintextSrc(t *testing.T) {
	src := models.MeshMembership{InMesh: false}
	dst := models.MeshMembership{InMesh: true, Mtls: mtls(models.MeshPermissive)}

	verdict := canReachSource().CanReach(src, dst, 0)

	if verdict.Verdict != "allow" {
		t.Fatalf("dst permissive: want allow, got %q (%s)", verdict.Verdict, verdict.Reason)
	}
}

// Dst STRICT, src outside mesh → deny with the "not in mesh" reason.
func TestCanReach_DstStrictSrcNotInMesh(t *testing.T) {
	src := models.MeshMembership{InMesh: false}
	dst := models.MeshMembership{InMesh: true, Mtls: mtls(models.MeshStrict)}

	verdict := canReachSource().CanReach(src, dst, 0)

	if verdict.Verdict != "deny" {
		t.Fatalf("want deny, got %q", verdict.Verdict)
	}
	if verdict.Reason == "" || verdict.Reason == "mesh permits" {
		t.Errorf("reason must explain src-side gap, got %q", verdict.Reason)
	}
}

// Dst STRICT, src in mesh but its mTLS is DISABLE — deny. The InMesh flag alone
// isn't enough; the src must actually speak mTLS.
func TestCanReach_DstStrictSrcMeshMtlsDisable(t *testing.T) {
	src := models.MeshMembership{InMesh: true, Mtls: mtls(models.MeshDisable)}
	dst := models.MeshMembership{InMesh: true, Mtls: mtls(models.MeshStrict)}

	verdict := canReachSource().CanReach(src, dst, 0)

	if verdict.Verdict != "deny" {
		t.Fatalf("src DISABLE against STRICT dst: want deny, got %q (%s)", verdict.Verdict, verdict.Reason)
	}
}

// Dst STRICT, src in mesh and STRICT too → allow. Baseline mesh-to-mesh.
func TestCanReach_BothStrict(t *testing.T) {
	src := models.MeshMembership{InMesh: true, Mtls: mtls(models.MeshStrict)}
	dst := models.MeshMembership{InMesh: true, Mtls: mtls(models.MeshStrict)}

	verdict := canReachSource().CanReach(src, dst, 0)

	if verdict.Verdict != "allow" {
		t.Fatalf("both strict: want allow, got %q (%s)", verdict.Verdict, verdict.Reason)
	}
}

// Dst STRICT, src in mesh with nil Mtls → allow. Only an explicit DISABLE
// verdict is a hard block; nil is treated as "not-disabled". Regression guard
// against a naive nil-deref that would panic.
func TestCanReach_DstStrictSrcMtlsNil(t *testing.T) {
	src := models.MeshMembership{InMesh: true, Mtls: nil}
	dst := models.MeshMembership{InMesh: true, Mtls: mtls(models.MeshStrict)}

	verdict := canReachSource().CanReach(src, dst, 0)

	if verdict.Verdict != "allow" {
		t.Fatalf("src in mesh with nil Mtls (not DISABLE): want allow, got %q (%s)", verdict.Verdict, verdict.Reason)
	}
}

// Both endpoints zero-value MeshMembership (both out of mesh) → allow. The
// documented "zero decodes as plaintext-src input" contract.
func TestCanReach_ZeroValueBothSides(t *testing.T) {
	verdict := canReachSource().CanReach(models.MeshMembership{}, models.MeshMembership{}, 0)

	if verdict.Verdict != "allow" {
		t.Fatalf("zero-value pair: want allow, got %q", verdict.Verdict)
	}
}
