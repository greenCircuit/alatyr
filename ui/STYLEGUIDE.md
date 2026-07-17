# UI Styleguide

Reference for future sessions touching `ui/`. Distilled from the DetailPanel redesign preview (`components/DetailPanel/DesignPreview/`), backend-sre + frontend-ux + ui-design-auditor consensus, and the fixes iterated on 2026-07-16/17.

Companion docs:
- `components/DetailPanel/REDESIGN.md` — what was tried, per-panel checklist.
- `components/DetailPanel/REDESIGN_GAPS.md` — what LIVE has that redesign dropped (rolling punch list).

Audience: expert operator. Density > friendliness. Assume reader knows SRC/DST, engine encoding, coverage terminology. No hand-holding on Kubernetes primitives.

---

## 1. Where tokens live

Single source of truth: `ui/src/style/colors.css` (`:root`). Every color goes through a `--color-*` var — never inline hex outside this file.

Component CSS modules alias into short local names via `.tokens { --allow: var(--color-allow); ... }` (see `DesignPreview/tokens.module.css`). Edit the palette in `colors.css`, alias in the module. Never invent a hex in a component.

Spacing scale: **4 / 8 / 12 / 16** only. Nothing between. `--gap-xs / -sm / -md / -lg` + `--pad-tier1 / -tier2`.

Radii: **3 / 6 / 8** — `--r-chip / -card / -panel`. Nothing else.

Motion: one duration, one easing across every state change — `--motion: 120ms cubic-bezier(0.2, 0.7, 0.3, 1)`. Don't add per-component curves.

---

## 2. Color system

Four axes, kept separate so spatial role never competes with semantic verdict.

| Axis | Tokens | Use |
|------|--------|-----|
| **Role** | `--color-role-src` (blue), `--color-role-dst` (amber) | SRC/DST pills, direction arrows (`↑ egress` src-tinted, `↓ ingress` dst-tinted), count capsules |
| **Engine** | `--color-engine-k8s / -istio / -mesh` | EngineChip, graph edge stroke |
| **Semantic** | `--color-allow / -deny / -warn / -caution / -info / -unknown` | Verdicts, state text, action icons |
| **Tinted bg** | `--color-allow-bg / -deny-bg / -warn-bg / -inert-bg` | Only where hierarchy §3 permits |

Ink on tinted surfaces: `--color-ink-tinted-*`. WCAG-safe — do NOT put white text on `--warn-bg`.

---

## 3. Surface hierarchy — CRITICAL

Two-tier system. Adding a third layer of tint is the single most common mistake.

**Tier 1** (`.tier1`) — group surface. `--tier1-bg` (`#ffffff08`), no border, 12px padding. Section wrapper: identity block, mesh block.

**Tier 2** (`.tier2`) — leaf card, nested one level. `--tier2-bg` (`#ffffff12`), 8px padding. Individual PolicyCard/PostureRow.

**Semantic surfaces** (`.semanticDeny / -Allow / -Warn`) — same `--tier2-bg` neutral fill + colored 3px left stripe. Used in lists (findings, per-engine verdicts, per-policy rows). Border-only tint carries state peripherally.

**Hero verdict** (`.verdictCallout`) — ONE per panel. Full tinted bg (`--allow-bg / -deny-bg / -warn-bg`) + 4px left stripe. This is the ONE place full-surface tint is allowed. Because it's singular, it doesn't create signal fatigue.

### Rules

- **Never wrap semantic surfaces or tier2 cards in another tier1/tier2**. Content flows under section headers (eyebrow text), not inside another box.
- **Max nesting**: `frameBody → details/section → PolicyCard`. Two visible filled surfaces below the frame. Anything more = wall of cards.
- **Never fill full bg with `--deny-bg` or `--allow-bg` on repeated elements**. Lists of red/green cards = signal fatigue. Every card the same color = no card stands out.

### Anti-pattern (what we killed)

```css
/* WRONG — full tint on every card in a list */
.policyCardBlocks { background: var(--deny-bg); }
.policyCardAllows { background: var(--allow-bg); }
```

```css
/* RIGHT — neutral bg, colored left stripe */
.policyCard { background: var(--tier2-bg); border-left: 3px solid transparent; }
.policyCardBlocks { border-left-color: var(--deny); }
.policyCardAllows { border-left-color: var(--allow); }
```

Exception: `.postureDenyAll` gets a full 1px `--deny` border (not just stripe) — hard blocks must read at scroll speed. Never scale this to more than the one exception.

---

## 4. Component recipes

Reference implementations live in `DesignPreview/panels.tsx`. Same shape must be used when porting into LIVE views.

### Hero verdict callout

Top of every panel. One-line answer + evidence chips underneath. Derived off the loudest signal (perEngine deny > worst status chip). Solves the LIVE silent-lie where a `warn` status chip renders while istio actively denies.

### PolicyCard

Neutral card + colored left stripe. Header: ActionIcon (✓/✗ colored square) + policy name + EngineChip + ns + optional cross-ns badge · right side: state word (colored) + ManifestLink. Body: direction row, ports, node-selector (how workload matched), peer list (collapsible over threshold), optional L7 block.

### PostureRow

One-line blanket-posture verdict (deny-all / allow-all / unenforced). Renders ABOVE per-peer PolicyCards for the same engine. Deny-all gets full `--deny` border, others get left stripe.

### MeshBlock

SA identity + effective mTLS mode + revision + sidecar status + principals + **PA chain w/ effective one marked** + **per-port mTLS overrides** + waypoint + mtls issues. Not-enrolled empty state renders explicitly ("not enrolled") — hiding is a silent lie (indistinguishable from "mesh data not fetched").

### PortsSummary (Compare panel)

Per-engine SRC-egress vs DST-ingress ports side-by-side. Surfaces "SRC opens 8080, DST accepts 9090" mismatch cases without unfolding rule cards.

### Blocker rows (Compare panel)

Semantic-deny card w/ RolePill · direction · PolicyRef · reason · **paSource** (PeerAuthentication culprit for mesh-mTLS blocks) · `deleteToAllow` subrule prose.

---

## 5. Silent-lie surfaces (must exist, no exceptions)

Silent lie = UI renders "no problem" or "no info" when the truth is "we couldn't tell". These are the 3am pager cases. Fixing them is the redesign's actual value proposition.

- **Staleness stamp** on frame header (`.stalenessStamp`). Informer-lag honesty: cached graph state may render stale reachability.
- **`srcUnavailable` / `dstUnavailable` caveats** in Compare per-engine grid. Distinguish "no policies match" from "we couldn't list" (RBAC / unfetched ns).
- **`reverseError` chip** on Compare bidirectional row (`⚠ reverse unavailable`). Distinguish "reverse allow" from "reverse unknown".
- **Unresolved-culprit `?` badge** on PolicyRef. Findings referencing a policy NOT in the resolvedPolicyKeys set (deleted / RBAC / unfetched ns) render dim w/ badge.
- **Wildcard peer explicit render** — `from: []` on ingress NetworkPolicy renders as `any peer (wildcard)`, not empty.
- **L7 negations** (`notHosts / notMethods / notPaths`) render in `--deny` w/ line-through. Silent Istio exclusion inside a permissive-looking rule = biggest L7 footgun.
- **Dead-letter port warning** on edge rows when policy port not in destination `listensOn`.
- **PA chain + port overrides** on MeshBlock. Strict-ns w/ permissive-port override was invisible before.
- **`paSource` on mesh-mTLS blockers**. Without it, blocker has no path to the manifest that caused it.
- **Not-enrolled mesh empty state**. Explicit "not enrolled" chip + reason, never hide the block.

If you add a new data field, ask: what does its absence look like? If absence ≠ "we don't know", add an unavailable state.

---

## 6. Layout patterns

- **Bootstrap utilities first** for layout/spacing/typography.
- **CSS module** (`.module.css`) for exact px, custom colors, z-index, calc, transitions Bootstrap can't express.
- **Never `style={{}}` for anything durable.** Inline style is a smell — flag and replace with a module class. Exception: one-off dynamic value (e.g. `style={{ color: severityColor }}`).
- **Never `<table>` for label/value info display**. Use `d-flex flex-column` with `<div>` label + `<div>` value pairs. Never `<span>` — inline element, unreliable in flex.
- **Stack label above value** (block divs). Side-by-side only when both are short AND explicitly requested.
- **Overlay panels on Cytoscape**: absolute positioned overlay > flex sibling. Flex resize races with canvas renderers.
- **Click node → panel opens**: re-fit viewport into the non-panel canvas area, don't rely on the user to pan.

---

## 7. Iteration mode

User works UI iteratively, not spec-driven. Expect many small tweaks on one element.

- **Don't re-read the same component between consecutive tweaks.** State is already known.
- **Don't propose unrelated improvements** while implementing a tweak. Stay in the lines.
- **Vague ask ("fix the panel")** → clarify with one concrete question, don't guess and edit.
- **No `npm run build`** — Vite dev server is always running and hot-reloads.
- **No feature-parity porting.** If a LIVE feature isn't in redesign, either (a) it doesn't fit the new hierarchy — cut it and doc why, or (b) it's a genuine gap — port carefully. Don't port every LIVE feature into redesign just to match; that turns redesign into a reskin.

---

## 8. Anti-patterns — DO NOT

- Wall of red/green cards (fill full bg on repeated policy/finding items).
- 4+ nested tinted boxes (frame → tier1 → tier2 → semantic → row).
- Inline `style={{}}` for anything reusable.
- `<table>` for label/value pairs.
- Emoji glyphs inside uppercase headings (extract to SVG/icon component instead).
- Custom motion curves per component.
- New hex colors in a component CSS file (add to `colors.css` `:root`).
- Silent absence when the truth is "unknown" — always add an unavailable state.
- Hiding a block when data isn't fetched — hiding reads identical to "no data", which is the lie.
- Backwards-compat shims, feature flags, or `// removed` breadcrumbs for cut features. Just delete.

---

## 9. Adding a new field to a panel

1. **What does absence mean?** If "we don't know" ≠ "no problem", add an unavailable/error state alongside the value type.
2. **Where does it fit the hierarchy?** Section header (eyebrow) → row w/ role/engine chip → value. Don't invent a new tier.
3. **Does it need color?** Only if it's a verdict/state. Otherwise mono text on `--tier2-bg`.
4. **Add token to `colors.css` if new color needed.** Alias in the component module.
5. **Update REDESIGN.md** if the field affected the redesign's checklist.

---

## 10. Genuine deltas from LIVE (protect these)

The redesign's value = these specific fixes, not the visual repaint. When merging into LIVE or iterating further, protect:

- Hero verdict derived from perEngine deny signal (not just worst-status chip).
- Staleness stamp on frame header.
- srcUnavailable / dstUnavailable / reverseError caveats.
- Unresolved-culprit `?` badge on PolicyRef.
- Wildcard peer explicit render.
- L7 negations styled distinctly.
- Dead-letter port warning.
- Wall-of-color drop (neutral bg + stripe).
- Nesting collapse (max 2 filled surfaces below frame).
- MeshBlock PA chain + port overrides + not-enrolled empty state.

Everything else is LIVE feature parity — port only if the operator would page someone without it.
