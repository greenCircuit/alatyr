# Branch `46-ui-bugs` — UI design audit

Audit of fixes applied for GitLab issue #46 (UI bugs). Scope: visual/craft
quality only, no code-correctness review. Bug #6 (mTLS permissive color) was
skipped per owner decision. Bug #3 in this audit refers to the IssuesDrawer
header pill restyle (the "wrong styling" from the issue text).

Files touched:

- `ui/src/components/DetailPanel/shared/edge-composites.tsx`
- `ui/src/components/DetailPanel/views/EdgeView.tsx`
- `ui/src/components/DetailPanel/views/ReachabilityView.tsx`
- `ui/src/components/DetailPanel/shared/StatusIcon.tsx`
- `ui/src/components/DetailPanel/shared/status.tsx`
- `ui/src/components/DetailPanel/shared/issues.tsx`
- `ui/src/components/IssuesDrawer/index.tsx`
- `ui/src/components/IssuesDrawer/IssuesDrawer.module.css`
- `ui/src/components/PolicyGraph/index.tsx`

Severity tags: **must** = ships broken, **should** = ships wrong, **nit** =
polish. Format: one line problem + one line fix per finding.

---

## Bug 1 — Reachability verdict + policy-rules eyebrow

**1a.** [nit] `edge-composites.tsx:115` — "Reachability: can reach" repeats the
section concept inside the hero. The label prefix reads as a category tag
bolted onto the verdict rather than part of it. `ReachabilityView`'s
`VerdictHeadline` (line 109) already lives at `.eyebrow` + hero — edge view is
now inconsistent with its sibling.
Fix: move `Reachability` to an eyebrow above the callout, keep the hero to
`→ can reach` / `✗ cannot reach`.

**1b.** [nit] `edge-composites.tsx:117` — `style={{ fontWeight: 600 }}` is an
inline style on a durable rule (deny path always gets 600).
Fix: move to a module class (extend `.reasonEmph` or similar).

**1c.** [should] `EdgeView.tsx:48` — `Policy rules (N) — allow/deny by rule`.
The em-dash annotation is doing pedagogical work the operator doesn't need
after first read. At eyebrow scale (10px/700/uppercase) the dash reads as
punctuation inside a label.
Fix: `Policy rules ({edges.length})` + move the annotation to a `title` attr.

---

## Bug 2 — Compare-nodes mesh chip icon

**Resolution (post-audit revision):** The generic shield glyph was replaced
with `EngineLogo` so the mesh chip carries the actual provider mark
(Istio sail, etc.) instead of a shared shield. Out-of-mesh state is signalled
by grayscale + reduced opacity on the same logo. `ShieldOffIcon` was removed
as dead code; `ShieldIcon` retained for the header mesh-overlay toggle.

**2a-orig.** [obsolete] Icon-size mismatch between 12px `ShieldIcon` and 10px
chip text. Superseded by the EngineLogo swap; `EngineLogo` is set to
`size={12}` which reads correctly against the 10px chip label (brand marks
tolerate the size bump better than a monochrome stroke glyph).

**2b.** [nit] Still applies — `.miniChip` is `inline-flex` but lacks
`align-items: center`; the call site patches via Bootstrap
`d-inline-flex align-items-center gap-1`. Fold `align-items: center` into
`.miniChip` and drop the utility.

**2c-orig.** [obsolete] `ShieldOffIcon` stroke-width imbalance. Component
removed.

---

## Bug 3 — IssuesDrawer severity counts

**Resolution (post-audit revision):** The three-pill header was replaced with
the project's canonical tier-count shape used by
`WorkloadsTable.IssueChip` — a single `btn btn-sm btn-dark border` element
wrapping colour-tinted `●count` entries per tier. `.sevPill` CSS removed as
dead code.

**3a.** [resolved] `.sevDot` migrated from character glyph `●` to a
background-filled inline-block circle in `DetailPanel.module.css:361`.
Callers (`issues.tsx:44`, `RiskyWorkloadsTable.tsx:58`, `IssuesTable.tsx:172`)
now render `<span className={s.sevDot} />` with no child glyph. No
font-dependent glyph anywhere in the severity-dot surface.

**3b-orig.** [obsolete] `color-mix()` support-floor comment. `.sevPill` rule
removed.

---

## Bug 4 — Partial-access findings excluded from workload verdict count

**4a.** No finding. `issueTier(issue.type) !== 'info'` at
`issues.tsx:106` is correct; the inline comment justifies the decision.

---

## Bug 5 — SeverityIcon prefix on status badges

**5a.** [must] `status.tsx:25` — `SeverityIcon` defaults to `size=12` inside
an 11px `.statusBadge`. Icon reads optically heavier than the badge text,
inverting hierarchy. `statusBadgeSymbol` is 9px, so the SVG dwarfs both symbol
and label.
Fix: `<SeverityIcon severity={cfg.severity} size={10} />` or define a
`badgeIconSize` constant.

**5b.** [should] Same file — after the change, the badge renders three
signal carriers: SVG icon + text symbol (`WAN↑`, `LAN⇆`, etc.) + label. The
symbol and the SeverityIcon are semantically redundant (both encode severity
tier) in different visual registers. The SVG is more legible at 10-12px than
a Unicode glyph.
Fix: drop `<span className={s.statusBadgeSymbol}>{cfg.symbol}</span>`.
`.statusBadgeSymbol` and the `symbol` field become dead — queue for removal.

**5c.** [should] `StatusIcon.tsx:75` — `warning`/`caution` map to
`ShieldIcon`, and `ShieldIcon` is also used in `MeshSideCard` for "in mesh".
Same glyph now carries two semantic roles: mesh membership and warning
severity. Operators scanning status badges may read a shield as
"mesh-related".
Fix: use `WarnIcon` (or a distinct outline variant) for warning/caution
severity so shield stays reserved for mesh membership.

---

## Bug 7 — Sort within severity bucket

**7a.** No finding. `IssueSection` groups keep `ALL_ISSUE_TYPES` order and
sort within-group by endpoint key. Correct.

**7b.** [nit] `IssuesDrawer/index.tsx:41` — `sortBucket` defined inline in
render, re-created every render. No visual defect; hoist to module scope
since it's pure.

---

## Bug 8 — Namespace-node selection fallback

**8a.** [nit] `PolicyGraph/index.tsx:348` — synthesized `WorkloadNode`
includes `statuses: []` but omits `statusesBySource` and any mesh membership.
If downstream panel code reads those fields without an optional guard the
namespace path can crash. Not a visual defect but a UX cliff.
Fix: audit `WorkloadView` and `MeshSideCard` for direct reads on those
optional fields before ns nodes ship as a supported selection target.

---

## Cut list (dead-code / follow-ups)

- `statusBadgeSymbol` span + CSS class (redundant with SeverityIcon)
- `symbol` field rendering in status badges — icon alone carries the signal
- Em-dash annotation in the edge-view eyebrow (move to `title`)
- Bootstrap `d-inline-flex align-items-center` on MeshSideCard miniChip once
  `.miniChip` gains `align-items: center`

---

## Overall

Direction is right — Bootstrap badges replaced with token-driven components,
pre-attentive icon encoding added. But icon sizing is unresolved across every
new insertion point (12px SVG inside 10-11px chips), and the shield glyph now
carries two conflicting semantic roles (mesh membership and warning severity)
that will silently erode the operator's ability to read the surface under
stress. Address 2a and 5a/5c before merge; the rest are polish for a follow-up
pass.
