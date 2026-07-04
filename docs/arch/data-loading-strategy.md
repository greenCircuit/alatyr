# Data-Loading Strategy: Backend-Computes, UI-Lazy-Loads

**Status:** Guideline (cross-cutting, not a single decision)
**Date:** 2026-07-01
**Applies to:** any new view or panel that displays derived/aggregated data

House style for deciding *where* a view's data is computed and *when* it is
fetched. Reference this before adding an endpoint or a frontend derivation.

## Default

**Backend computes the aggregate; the UI lazy-loads it per view and caches it in
the store.**

The backend owns truth — rules, workloads, selectors, mesh state all live in Go.
Any view that is a real aggregation of that truth is computed where the truth is,
so there is one authoritative site and nothing is reconstructed in JS.

Why this is the default here:

- **Single source of truth.** One computation site. No second implementation in
  the frontend that can drift from the Go one.
- **Pay for what's viewed.** Detail panels, tables, reachability — none needed
  until the user asks. Fetch on interaction, keeps first paint cheap on a big
  cluster.
- **Thin, evolvable contract.** UI asks for a shape; backend returns exactly that
  shape. A new column is a new field on the response struct — no payload
  widening, no new endpoint, no second computation site.

## When NOT to go to the backend

**Data already shipped + strict projection.** If the answer is a pure reshape of
bytes the browser already holds (e.g. counts derivable from edges already in the
store), a round trip is waste — compute client-side. The only reason to still go
backend in that case is avoiding two computation sites, not the data itself.

**Latency-critical interaction.** Hover tooltips, typeahead, anything with a
sub-100ms expectation. A round trip per hover/keystroke feels broken — prefetch
or ship the data inline.

**Chatty N+1.** Lazy-per-row that fires one request per visible element is death
by round trips. Lazy must be per-*view* (one call fills the table), never
per-*element*.

## Decision rule

```
Is the answer a strict reshape of data the client already has?
  yes → compute client-side (no round trip)
  no  → backend computes, UI lazy-loads per view

Is the interaction latency-critical (hover / keystroke)?
  yes → prefetch / inline, don't lazy-load on demand
```

## Discipline that keeps it clean

- **One call per feature/view**, not per row, not per field. Cache the result in
  the store keyed by whatever scopes it (nodeId, filter set, namespace subset).
- **Growth stays cheap.** Adding data later = the endpoint grows a field, the
  store cache key is unchanged, the component reads more. No new round trip
  shape.
- **Backend returns the view's shape, not raw truth.** Aggregate server-side and
  return what the view renders; don't ship raw rules and re-aggregate in JS.

## Honesty constraint (carries over from the badges/reachability work)

An aggregate must not claim more than it computed. Counts of *policy references
between endpoints* are not *effective reachability* (which needs lock state, deny
subtraction, and mesh mTLS — see `reachability-plan.md`). Name the column for
what it is. A number that reads as a reachability verdict but is only adjacency
is worse than no view.
