# Vendored: Calico selector parser (`libcalico-go/lib/selector`)

Copied source, **not** a Go module dependency. Package name is `selector`
(dir is `calicoselector` for provenance) — import it aliased:

```go
import selector "alatyr/internal/thirdparty/calicoselector"

sel, err := selector.Parse(`kubernetes.io/metadata.name in {"gitlab","kube-system"}`)
match := sel.Evaluate(map[string]string{"kubernetes.io/metadata.name": "kube-system"}) // true
```

Consumed by `internal/policy/calico` (`parseSelector`) to evaluate
GlobalNetworkPolicy `selector` / `namespaceSelector` expressions against
workload and namespace labels.

## Why vendored instead of imported

The Calico selector grammar (`all()`, `has()`, `==`, `!=`, `in {…}`,
`not in {…}`, `&&`, `||`, `!`, nested parens) is a real lexer + parser. We need
the **real** one — a hand-rolled parser that gets negation or precedence wrong
silently mis-renders reachability, which is the exact failure this project must
not ship. But the upstream package **cannot be `go get`-imported**, for three
reasons, worst last:

1. **`+incompatible` module layout.** `github.com/projectcalico/calico` has no
   `/v3` path suffix but tags `v3.x`, so `@latest` resolves to `v3.21.1` (2021),
   where this package doesn't exist at this path. Current code lives only on the
   default branch.
2. **The default branch pins an unresolvable dep.** It requires
   `k8s.io/kube-openapi@v0.31.0`, which `proxy.golang.org` reports as an unknown
   revision — the module graph never closes.
3. **The parser depends on an unpublished nested module (the killer).**
   `parser/{ast,parser,stringset}.go` import
   `github.com/projectcalico/calico/lib/std/uniquestr`. In the Calico monorepo
   `lib/std` is a *separate* nested module glued in via a local
   `replace github.com/projectcalico/calico/lib/std => ./lib/std` directive.
   `replace` directives apply only to the module being built, never to external
   consumers, and `lib/std` was never published standalone (`proxy.golang.org`
   has no versions for it). So `go get` of the selector package can never
   resolve `lib/std` — import is impossible **by construction**, at any version
   or toolchain.

Copying the files is the only route to the real parser. What copying gives up
(version tracking, `go get -u`, CVE signal) was never available anyway — there
is no consumable library to track.

## Source

- Repo: `github.com/projectcalico/calico`
- Commit: **`3d00673793c8`** (default-branch pseudo-version
  `v1.11.0-cni-plugin.0.20260724211350-3d00673793c8`)
- Fetched: 2026-07-26
- License: **Apache-2.0** (see `LICENSE`). Tigera copyright headers preserved
  in every file per §4.

## Local modifications

Files are otherwise **byte-identical to upstream** — keep them that way so
re-sync stays a mechanical re-copy:

- **Import paths rewritten** `github.com/projectcalico/calico/...` →
  `graph/internal/thirdparty/calicoselector/...` (see mapping in "How to
  update").
- **`github.com/sirupsen/logrus` kept as-is** and added as a normal go.mod
  dependency. It resolves independently of the broken Calico module graph, and
  keeping the upstream debug-logging lines untouched preserves byte-fidelity.
  Do **not** strip it — stripping diverges from upstream and complicates diffs.

## Layout

| Path | Package | Upstream origin |
|------|---------|-----------------|
| `selector.go` | `selector` | `libcalico-go/lib/selector` (`Parse`, `Validate`, `Normalise`) |
| `parser/{ast,parser,stringset}.go` | `parser` | `libcalico-go/lib/selector/parser` |
| `tokenizer/{tokenizer,kind_string}.go` | `tokenizer` | `libcalico-go/lib/selector/tokenizer` |
| `hash/unique_id.go` | `hash` | `libcalico-go/lib/hash` |
| `uniquestr/unique_string_handle.go` | `uniquestr` | `lib/std/uniquestr` (fetched from GitHub — excluded from the module zip) |

Test files (`*_test.go`) were intentionally not copied.

## How to update

1. Check out the target Calico commit; copy the eight non-test files above into
   the same layout here. Note the `uniquestr` file is not in the module zip —
   fetch it from GitHub raw at the commit SHA.
2. Re-apply the import-path rewrite:
   ```
   github.com/projectcalico/calico/libcalico-go/lib/selector/parser    → graph/internal/thirdparty/calicoselector/parser
   github.com/projectcalico/calico/libcalico-go/lib/selector/tokenizer → graph/internal/thirdparty/calicoselector/tokenizer
   github.com/projectcalico/calico/libcalico-go/lib/hash               → graph/internal/thirdparty/calicoselector/hash
   github.com/projectcalico/calico/lib/std/uniquestr                   → graph/internal/thirdparty/calicoselector/uniquestr
   ```
3. Update the **Commit** and **Fetched** fields above.
4. `go build ./... && go test ./internal/thirdparty/calicoselector/...`.

> 3am note: this is frozen source. It gets no automatic grammar-change or CVE
> signal. If a GlobalNetworkPolicy uses a selector operator this copy rejects,
> re-sync from a newer Calico commit is the fix.
