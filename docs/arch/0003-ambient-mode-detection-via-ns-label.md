# ADR 0003: Ambient-Mode Mesh Detection via Namespace Label

**Status:** Accepted
**Date:** 2026-05-19
**Branch:** `2-add-istio-l3-l4-authorization-policy-support`

## Context

Istio `AuthorizationPolicy` only enforces on workloads that are in the mesh. A policy whose selector matches a workload outside the mesh has zero runtime effect. The visualizer must distinguish in-mesh from out-of-mesh workloads so the intersection logic and status keys are correct.

The user's cluster runs **Istio ambient mode** (no sidecars; ztunnel handles L4, waypoints handle L7). Mesh membership in ambient mode is signaled by the namespace label `istio.io/dataplane-mode=ambient`. Per-pod opt-out is possible via `istio.io/dataplane-mode=none` on the pod but is not used in this deployment.

## Decision

Mesh membership is determined by the namespace label `istio.io/dataplane-mode=ambient`. A workload is in-mesh iff its namespace carries that label.

Per-pod overrides (`istio.io/dataplane-mode=none` on individual pods) are **not checked in this branch**. Documented as a known gap.

## Alternatives considered

**A. Detect via pod spec (`istio-proxy` container present).**

Rejected — works only for sidecar mode. Ambient mode has no sidecar. Wrong answer for this user's cluster.

**B. Combine NS label + per-pod annotation + sidecar container.**

More accurate. Rejected as over-engineered for the stated deployment. The combined check belongs in a future revision if a user reports needing per-pod overrides or runs sidecar mode.

**C. Assume every pod is in-mesh.**

Rejected — gives wrong intersection results in any cluster with a mix of meshed and non-meshed namespaces (the common case during ambient rollouts).

## Consequences

**Positive:**
- One label lookup per workload. Cheap.
- Aligns with how Istio operators conventionally enable ambient (NS-scoped opt-in).
- Drives the `istio.AuthPolicyOnOutOfMeshWorkload` status key cleanly: if an `AuthorizationPolicy` selects a workload but the workload's NS lacks the ambient label, flag it.

**Negative:**
- Per-pod opt-outs not detected. A pod labeled `istio.io/dataplane-mode=none` inside an ambient namespace will still be treated as in-mesh by the visualizer. Documented gap — revisit if a user reports a misleading graph.
- Sidecar-mode clusters not supported by this detection. Documented gap. Adding sidecar-mode support is a focused change: extend the Istio source's `inMesh()` helper with a second check.

## Implementation note

The check lives in `internal/policy/istio/source.go`, not in `internal/graph/`. Mesh membership is an Istio-source concern; other sources (K8s NP, Calico, Cilium) do not care.
