# ADR 0004: L3/L4-Only Scope for Istio AuthorizationPolicy

**Status:** Accepted
**Date:** 2026-05-19
**Branch:** `2-add-istio-l3-l4-authorization-policy-support`

## Context

`AuthorizationPolicy` covers both L3/L4 (namespaces, principals, IP blocks, ports) and L7 (HTTP paths, methods, hosts, headers, JWT). The visualizer's existing graph model is built around L3/L4 reachability — edges are workload-to-workload on a port, not method-to-path. Mixing L7 fields into the same edge would require a substantially richer edge model and a richer UI.

## Decision

Scope this branch to L3/L4 fields only:

- `spec.selector.matchLabels`
- `spec.action`: `ALLOW` and `DENY` only
- `spec.rules[].from[].source`: `namespaces`, `principals`, `ipBlocks`, `notNamespaces`, `notPrincipals`, `notIpBlocks`
- `spec.rules[].to[].operation`: `ports`, `notPorts`
- Root-namespace (`istio-system`) policies treated as mesh-wide
- `AUDIT` and `CUSTOM` actions excluded from tuple computation

## Out of scope (documented gaps)

These are explicitly **not** handled by this branch. Behavior in their presence:

| Item | Behavior |
|---|---|
| L7 fields (`paths`, `methods`, `hosts`, `headers`, `requestPrincipals`) | Silently ignored. A policy that uses only L7 fields produces no edges and may be misleading. |
| `PeerAuthentication` / mTLS mode | Not read. `principals` matches are assumed to succeed when source workload is in-mesh. Under `PERMISSIVE` mTLS with plaintext traffic, real Istio would reject principal-based rules — the graph will over-report reachability. |
| `AUDIT` action | Skipped from tuple computation. No status key. AUDIT has no traffic effect so this is semantically correct. |
| `CUSTOM` action | Cannot be statically resolved (delegates to external authz). Skipped from tuple computation. Surfaced via `istio.CustomAuthzUnresolved` status key so users know not to trust the graph for those workloads. |
| Ambient **egress** policy via waypoints | Not modeled. Egress is treated as K8s-only. |
| `requestPrincipals` (JWT) | L7-equivalent. Not read. |
| `when` conditions referencing L7 attributes (`request.headers`, etc.) | Not parsed. |
| `when` conditions on L3/L4 attributes (`source.ip`, `connection.sni`, `destination.port`) | Not parsed in this branch. Could be added without engine changes — open as a follow-up. |

## Reasoning

The graph's value is showing **L3/L4 reachability** — which workloads can establish a connection to which other workloads on which port. L7 authz is a different question ("what HTTP verbs can workload A call on workload B's `/api/*` paths") and would require:

- Different edge granularity (per-method, per-path)
- A different node model (services with route maps)
- A different filter UX

That's a separate, larger piece of work. Carving it out keeps this branch shippable and the model coherent.

## Consequences

**Positive:**
- Edge model unchanged in shape — only `contributors` field added.
- L3/L4 intersection is well-defined and unit-testable with fixtures.
- Known gaps are explicit and visible in the docs; users can decide whether the visualizer's view is sufficient for their environment.

**Negative:**
- A policy with mixed L4 + L7 rules is partially honored. The L4 portions contribute to edges; the L7 portions are silently dropped. A user reading the graph could assume the L7 restrictions don't exist. **Mitigation:** future work could add an `istio.PolicyHasL7Fields` status key on workloads whose AuthPolicies contain L7 fields, so users know to consult Istio config directly for those.
- `PeerAuthentication` blind spot can cause over-reporting in `PERMISSIVE` mTLS environments. Documented; revisit if reports come in.
