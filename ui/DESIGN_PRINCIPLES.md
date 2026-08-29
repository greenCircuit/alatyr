# Design Principles

Voice of the DetailPanel redesign. Explains WHY the surface looks the way it does — the aesthetic direction the token system carries but does not state. Read this before touching pixels.

Companion to:
- `STYLEGUIDE.md` — tokens, tier system, silent-lie surfaces, anti-patterns.
- `components/DetailPanel/REDESIGN.md` — per-panel checklist.
- `components/DetailPanel/REDESIGN_GAPS.md` — LIVE-vs-mockup punch list.

---

## 1. References

Not styled from taste — borrows from four tools operators already trust at 3am. Match these when in doubt.

- **Linear** — frame chrome, 8/12/16 rhythm, one-motion-curve calm, hover states intentional not decorative (`tokens.module.css:57` single `--motion`, `panels.tsx:14` `IconClose` stroke glyph). Borrow: chromeless surfaces, restraint on hover chrome.
- **Datadog APM detail drawers** — verdict banner on top, evidence stacked underneath, monospace for data identifiers (services, ports, labels). Borrow: verdict-first pane, mono-for-data discipline (`panels.tsx:405` `labelValue`).
- **Grafana Explore / Loki labels** — `{key="value"}` chip pattern with dim key + bright value, click-to-copy for `kubectl -l`. Borrow: `LabelChip` shape (`tokens.module.css:387`), `labelEq` as separator glyph not character.
- **tmux/htop status bars & Prometheus 3 UI** — dense text, colored dots + stripes for state, no wasted vertical space, monospace for tabular data. Borrow: `roleDot` (`tokens.module.css:156`), semantic stripes (`tokens.module.css:84`), 11px `eyebrow` (`tokens.module.css:133`).
- **Vercel dashboard** — role-tinted pills (SRC/DST), engine chips as tinted-bg+dot combos, subtle backdrop-blur frame. Borrow: `.frame` blur (`tokens.module.css:236`), `.engineChip` tint recipe (`tokens.module.css:187`).

**Not references:** any Bootstrap admin theme, any 2018-era k8s dashboard, anything with `.badge bg-primary`.

---

## 2. Creative principles

- **Verdict-first, evidence-second.** Hero callout at panel top answers the pager question in six words. Everything below is proof. `panels.tsx:664` (Node hero), `panels.tsx:937` (Edge). If the eye lands anywhere else first, the panel is broken.
- **Peripheral color, central neutrality.** State lives in the frame — 3px left stripe (`tokens.module.css:86`), colored dot, colored word — not in the fill. Reading surface stays `--tier2-bg` neutral so long text remains scannable. Full tint (`--deny-bg` / `--allow-bg`) is legal exactly once per panel (`.verdictCallout`). Repeat it and every card looks the same, which means no card stands out.
- **Type as hierarchy, not weight.** Three sizes carry the panel: 11px eyebrow, 13px body, 15px section, 18px hero (`tokens.module.css:39-42`, `:133`). Bold is a scalpel, not a chorus. Never solve hierarchy with heavier weight when a size step or color demotion would do it.
- **Mono for data, sans for prose.** Anything an operator would paste into `kubectl` — labels, ports, SAs, namespaces, policy names — renders monospace (`tokens.module.css:405`, `:473`, `:577`). Prose (verdict text, tooltips, findings copy) stays sans. Single loudest cue that separates this UI from a Bootstrap admin.
- **Chips are click targets, not decorations.** `LabelChip` copies `k=v`; `labelCopyAll` copies full selector; `PolicyRef` opens YAML; `openReachButton` re-scopes reachability. Every chip does work. If it does not, it should be text (`panels.tsx:141`, `:159`, `:78`).
- **Gap over rule.** Separation is done with `gap: 8px` on a flex column, not `border-top: 1px solid`. Dividers appear only when a semantic edge is being drawn (frame header, stripe on a state card). `.peerRow` (`tokens.module.css:561`) — no border, spacing does the work.
- **Absence is a value.** Silent-lie surfaces are a design principle, not a data concern. Empty state renders w/ chip + reason (`.notEnrolled`, `srcUnavailable`, `paChainRow` dim). Hiding a block reads identical to "no problem". Always show the shape of the thing that is missing.
- **One motion curve, one duration.** `120ms cubic-bezier(0.2, 0.7, 0.3, 1)` everywhere (`tokens.module.css:57`). Never per-component ease. Consistency of motion is what makes a surface feel like a tool instead of a collage.

---

## 3. Rules against the bootstrappy look

"Bootstrappy" = defaults leaking through. Bootstrap classes composed with tokens still look like Bootstrap. Mockup replaces Bootstrap idioms with purpose-built primitives; live views must too.

1. **Kill `badge bg-secondary` / `badge bg-info text-dark` for label + port chips.** Use `.labelChip` (mono key + dim `=` + bright value) and port-styled chips (info-tinted mono). Bootstrap badges are pill-radius, pill-padded, non-monospace — read as generic tags not typed data. Regressions: `rows.tsx:39, 73, 81, 89, 161, 186, 405`.
2. **Kill `border-top border-secondary` as section divider.** Use `gap: var(--gap-md)` on flex column, or `.eyebrow` (11px uppercase, 0.04em tracking, dim). Rules between items = admin table. Regressions: `rows.tsx:191, 255, 300, 410`.
3. **Kill Bootstrap `.card` shell + shadow.** Two-tier system: `.tier1` outer group (subtle tint, no border, 12px), `.tier2` leaf (darker tint, 8px), semantic stripe when state matters. No box-shadow beneath tier2. `.frame` is the only shadow surface (`tokens.module.css:242`).
4. **Kill `btn btn-link` for disclosures.** Use `<details>` + `.disclosureSummary` w/ `▸ / ▾` chevron that rotates on open (`tokens.module.css:353`). Bootstrap link-buttons render blue-underline-on-hover — wrong register for a tool.
5. **Type scale is 11 / 13 / 15 / 18, not 12 / 14 / 16 / 20.** Bootstrap defaults have no eyebrow tier; adopting theirs collapses hierarchy into one register. Body sits at 13, not 14 — density matters at hundreds of rows.
6. **Mono the data columns.** Every namespace, port, label value, policy name, SPIFFE ID, mTLS mode is `ui-monospace, SFMono-Regular, Menlo`. Prose stays sans. Mixing them inside a chip (dim sans key + mono value) is signature of the mockup — `.labelChip` (`tokens.module.css:387`).
7. **State lives on a 3px left stripe, never on the fill of a list item.** `.policyCardBlocks / -Allows / -Unenforced` (`tokens.module.css:498`). Repeated full tint = wall-of-red signal fatigue. Exception: `.postureDenyAll` (`tokens.module.css:526`) — full 1px deny border for hard blocks.
8. **Colored dots and tinted-bg chips beat outlined pills.** `.engineChip` = 6px dot + 14% color-mix bg + 11px semibold text (`tokens.module.css:187`). Never `border: 1px solid` chip — outlines add chrome without signal in dark UI.
10. **Radii are 3 / 6 / 8 only.** Chips at 3, cards at 6, panel at 8 (`tokens.module.css:52`). Bootstrap default `rounded` (0.375rem) is between the two — inconsistent radii break the visual chain chip → card → panel.
11. **Ghost buttons, not filled buttons, for secondary actions.** `.ghostButton` and `.openReachButton` (`tokens.module.css:309, :779`) — transparent bg, 1-alpha border, subtle role-tint on hover. Only ONE primary action per panel (`Pin as source`) gets filled `.primaryButton`.
12. **No inline `style={{}}` for anything durable.** Mockup uses inline style only for dynamic values (severity color from data). Everything static lives in `tokens.module.css`. Inline style is a smell — port-once, then it becomes a token.
13. **Bootstrap for layout, tokens for chrome.** Layout utilities are fine — `d-flex`, `flex-column/wrap`, `justify-content-*`, `align-items-*`, `gap-*`, `mb-*/mt-*/me-*/ms-*/p-*/py-*/px-*`, `text-break/truncate/center/end`, `min-w-0`, `flex-shrink-0/grow-1`. Layout classes are geometry, do not carry visual style. Bootstrap components that fit (nav tabs, dropdown positioning) also OK.
    **Always kill for chrome:** `badge bg-*` (→ `.chip-*` tokens), `border border-secondary` / `border-top border-secondary` (→ `.card` stripe or `gap`), `text-{secondary,danger,success,warning} small` (→ tokens), `bg-{dark,info,warning,primary,success,danger}` (→ color tokens), `btn btn-outline-*`, `btn btn-link`, `btn-close` (→ `.disclosureButton`/`.ghostButton`), `rounded shadow` on floating panels (→ `.dropdown-shell`).
    **Grey area, case by case:** `text-secondary` alone (Bootstrap `#6c757d` ≈ our `.dim` `#7a848e`), `fw-semibold`/`fw-bold` as one-offs (prefer `.section`/`.hero` tokens which set weight), `.fs-11`/`.fs-12` project custom (same as `.smallText`, worth converging).

---

## 4. Edge-panel LabelStrip decision

**Fold labels behind disclosure on Edge, inline on Node.**

**Reasoning.** Edge answers "can SRC reach DST, and why not?" — verdict, blocking policy, and port match carry that. Labels are the *selector* for policies, not the *outcome*. Rendering both endpoints' full label strips inline creates a 4-line wall that pushes actual policy evidence below the fold. On Node, labels are primary identity join key (operator's next move is `kubectl -l` from that workload) — they earn line-2 real estate. On Edge, one abstraction removed.

**Concrete shape.** Endpoint card shows: RolePill + name + `<details>` disclosure `▸ N labels · copy selector`, expanded by default when endpoint has fewer than 3 non-system labels, collapsed otherwise. Preserve `copy selector` button in collapsed summary — operator grabs selector without expanding. Namespace + type stay inline (they are the disambiguator, not the join key).

**Do not** hide labels behind modal or hover card — both break copy-paste flow. Disclosure keeps them one click away and one Cmd-F away.
