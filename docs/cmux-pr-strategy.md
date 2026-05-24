# CMUX Branch PR/Merge Strategy

**Branch:** `feature/cmux-operator-v1`
**Base:** `master`
**Commits:** 14
**Net change:** +7,836 lines across 16 files (all additive)
**Prepared:** 2026-05-24

---

## 1. Current Branch Inventory

### Commit map

| Hash | Message | Files touched |
|---|---|---|
| `743e1c9` | Add initial CMUX runtime reader | `reader.go`, `render.go`, `model.go`, `main.go`, `cmux_test.go`, `docs/cmux-operator.md`, `docs/cmux-roadmap.md`, `docs/cmux-runtime-reader.md` |
| `ca9ff6d` | Improve CMUX read-only detail rendering | `main_test.go` only |
| `94bc384` | Improve CMUX read-only detail rendering | `render.go`, `cmux_test.go` |
| `9a68138` | Add CMUX one-shot render options | `model.go`, `render.go`, `main.go`, `cmux_test.go` |
| `572a3c5` | Add CMUX task filtering | `model.go`, `render.go`, `main.go`, `cmux_test.go` |
| `a75aafd` | Add CMUX markdown report output | `render_markdown.go` *(new)*, `model.go`, `render.go`, `main.go`, `cmux_test.go` |
| `2772f7d` | Add CMUX JSON report output | `render_json.go` *(new)*, `model.go`, `render.go`, `main.go`, `cmux_test.go` |
| `f4edcee` | Document CMUX report usage | `main.go`, `main_test.go`, `docs/cmux-runtime-reader.md` |
| `0493f9e` | Polish CMUX Phase 1 documentation | `docs/cmux-runtime-reader.md`, `model.go`, `render.go` |
| `857807f` | Add CMUX review packet mode | `render_review.go` *(new)*, `model.go`, `render.go`, `main.go`, `cmux_test.go` |
| `d99cc92` | Design CMUX Phase 2 report workflows | `docs/cmux-phase2-design.md` *(new)* |
| `223d3df` | Add CMUX handoff packet mode | `render_handoff.go` *(new)*, `model.go`, `render.go`, `main.go`, `cmux_test.go` |
| `a3cf3f6` | Add CMUX merge readiness report | `render_merge.go` *(new)*, `model.go`, `render.go`, `main.go`, `cmux_test.go` |
| `417db3d` | Add CMUX blocked task report | `render_blocked.go` *(new)*, `model.go`, `render.go`, `main.go`, `cmux_test.go` |

### Logical feature groups

| Group | Commits | Description |
|---|---|---|
| **Core reader** | `743e1c9` | Package setup, `Snapshot`, `Read`, `NativeFetcher`, initial text render, `brevity cmux` CLI entry point |
| **Detail polish** | `ca9ff6d`, `94bc384` | Render detail improvements: worktree/prompt/last-run rows, extended test coverage |
| **Render options** | `9a68138`, `572a3c5` | `RenderOptions` (section, limit, task slug, state filter), CLI flags, filtering tests |
| **Output modes** | `a75aafd`, `2772f7d` | Markdown renderer (`render_markdown.go`), JSON renderer (`render_json.go`), `--output` flag |
| **Phase 1 docs** | `f4edcee`, `0493f9e` | `writeCmuxUsage`, `main_test.go` help coverage, `cmux-runtime-reader.md` reference docs |
| **Review packet** | `857807f` | `--review <slug>` mode, `render_review.go`, review checklist, merge/cleanup readiness |
| **Phase 2 design** | `d99cc92` | `docs/cmux-phase2-design.md` — design doc only, no code |
| **Handoff packet** | `223d3df` | `--handoff` mode, `render_handoff.go`, ranked task list, review candidates |
| **Merge report** | `a3cf3f6` | `--merge-report` mode, `render_merge.go`, six-group merge classification |
| **Blocked report** | `417db3d` | `--blocked-report` mode, `render_blocked.go`, four-group block classification |

---

## 2. Recommended PR Slicing

### Options considered

#### Option A — One large PR
Merge all 14 commits as a single PR.

**Pros:** single review context, no rebase risk between PRs, simpler logistics.

**Cons:** 7,836 lines across 16 files makes meaningful review difficult. Phase 1 infrastructure (reader, text/markdown/json, filtering) and Phase 2 reports (handoff, merge, blocked) serve different purposes and reviewers may want to validate the stable base before the Phase 2 surface area.

**Risk:** Any conflict on `main.go` or `render.go` from concurrent master work requires resolving across the entire diff.

#### Option B — Two PRs (Phase 1 core + Phase 2 reports) ✅ Recommended

Split at the natural Phase 1/Phase 2 boundary:

- **PR-1 (Phase 1):** commits `743e1c9` → `857807f` (10 commits) — reader, all output modes, filtering, review packet, Phase 1 docs
- **PR-2 (Phase 2):** commits `d99cc92` → `417db3d` (4 commits) — Phase 2 design doc, handoff, merge report, blocked report

**Pros:**
- PR-1 establishes the stable, tested foundation. Once merged, PR-2 applies cleanly on top.
- Reviewers can verify that the core infrastructure is sound before evaluating the report extensions.
- The split is exactly where `render_review.go` was added and `docs/cmux-phase2-design.md` was written — a pre-existing logical checkpoint in the commit history.
- Phase 2 PR-2 diffs are entirely additive: four new `render_*.go` files plus incremental additions to `model.go`, `render.go`, and `main.go`.

**Cons:** Requires sequenced merges; PR-2 cannot go in before PR-1 lands.

#### Option C — Three PRs
Further split Phase 1 into core (reader + text) and extended (markdown + JSON + docs).

**Not recommended.** The output modes are tightly coupled through `RenderOptions` and the dispatch chain in `render.go`. Splitting them creates an intermediate state where `--output markdown` doesn't exist yet, which is harder to validate and adds a third sequencing dependency for no meaningful review benefit.

---

## 3. Suggested PR Sequence

### PR-1: CMUX Phase 1 — Read-only operator report

**Title:** `feat(cmux): add read-only CMUX operator report (Phase 1)`

**Commits in scope:**

```
743e1c9  Add initial CMUX runtime reader
ca9ff6d  Improve CMUX read-only detail rendering
94bc384  Improve CMUX read-only detail rendering
9a68138  Add CMUX one-shot render options
572a3c5  Add CMUX task filtering
a75aafd  Add CMUX markdown report output
2772f7d  Add CMUX JSON report output
f4edcee  Document CMUX report usage
0493f9e  Polish CMUX Phase 1 documentation
857807f  Add CMUX review packet mode
```

**Files introduced:**

| File | Status | Description |
|---|---|---|
| `internal/cmux/reader.go` | New | `Read`, `Fetcher`, `NativeFetcher`, `Snapshot` |
| `internal/cmux/model.go` | New | `RenderOptions`, `OutputMode`, section/limit/filter fields, `ReviewTask` |
| `internal/cmux/render.go` | New | `Render` dispatch, text renderer, section/filter/limit logic |
| `internal/cmux/render_markdown.go` | New | GFM renderer |
| `internal/cmux/render_json.go` | New | JSON renderer, `CMUXReport`, JSON helper builders |
| `internal/cmux/render_review.go` | New | `--review` mode, checklist, readiness notes |
| `internal/cmux/cmux_test.go` | New | ~2,100 lines of in-memory tests |
| `cmd/brevity/main.go` | Modified | `brevity cmux` entry, `parseCmuxOptions`, `routeCmuxCommand`, `writeCmuxUsage` |
| `cmd/brevity/main_test.go` | Modified | Help output coverage |
| `docs/cmux-operator.md` | New | User-facing operator guide |
| `docs/cmux-roadmap.md` | New | Phase roadmap |
| `docs/cmux-runtime-reader.md` | New | Contract and architecture reference |

**Net change:** approximately +4,600 lines

**Risk:** Low.
- All new files; no existing functionality modified.
- `main.go` adds a new `commandCmux` branch; no existing command paths are changed.
- Every added flag uses `parseCmuxOptions`, isolated from other command parsers.

**Validation before merge:**
```
go test ./internal/cmux
go test ./cmd/brevity
go run ./cmd/brevity cmux
go run ./cmd/brevity cmux --output markdown
go run ./cmd/brevity cmux --output json
go run ./cmd/brevity cmux --section tasks --state reviewing
go run ./cmd/brevity cmux --review <any-slug>
git diff --check master
```

---

### PR-2: CMUX Phase 2 — Operator report extensions

**Title:** `feat(cmux): add Phase 2 operator reports (handoff, merge, blocked)`

**Depends on:** PR-1 merged to master.

**Commits in scope:**

```
d99cc92  Design CMUX Phase 2 report workflows
223d3df  Add CMUX handoff packet mode
a3cf3f6  Add CMUX merge readiness report
417db3d  Add CMUX blocked task report
```

**Files introduced:**

| File | Status | Description |
|---|---|---|
| `internal/cmux/render_handoff.go` | New | `--handoff` mode, ranked task list, review candidates, safety attestation |
| `internal/cmux/render_merge.go` | New | `--merge-report` mode, six-group merge classification |
| `internal/cmux/render_blocked.go` | New | `--blocked-report` mode, four-group block classification |
| `docs/cmux-phase2-design.md` | New | Phase 2 design doc (documentation only) |

**Files modified (additive only):**

| File | Change |
|---|---|
| `internal/cmux/model.go` | +`Handoff bool`, `MergeReport bool`, `BlockedReport bool` to `RenderOptions` |
| `internal/cmux/render.go` | +3 dispatch branches at the top of `Render()` |
| `internal/cmux/cmux_test.go` | +~1,370 lines (22 handoff + 16 merge + 20 blocked tests) |
| `cmd/brevity/main.go` | +3 CLI flags, +3 struct fields, +3 routes, +usage lines |

**Net change:** approximately +3,250 lines

**Risk:** Low to medium.
- All new `render_*.go` files are isolated; they cannot affect existing render paths.
- The three additions to `render.go`'s dispatch chain are `if opts.X { return }` guards at the top — they are unreachable unless the corresponding flag is set.
- The only merge risk is if master received other changes to `main.go` between PR-1 merge and PR-2 open. The `parseCmuxOptions` function would be the merge surface. It is confined to one function (~40 lines) and the additions are flag registrations and struct field assignments.

**Validation before merge:**
```
go test ./internal/cmux
go test ./cmd/brevity
go run ./cmd/brevity cmux --handoff
go run ./cmd/brevity cmux --handoff --output markdown
go run ./cmd/brevity cmux --handoff --output json
go run ./cmd/brevity cmux --merge-report
go run ./cmd/brevity cmux --merge-report --output markdown
go run ./cmd/brevity cmux --merge-report --output json
go run ./cmd/brevity cmux --blocked-report
go run ./cmd/brevity cmux --blocked-report --output markdown
go run ./cmd/brevity cmux --blocked-report --output json
git diff --check master
```

---

## 4. Merge/Conflict Risk Analysis

### Per-file conflict exposure

| File | PRs that touch it | Risk | Notes |
|---|---|---|---|
| `internal/cmux/render.go` | Both PRs | **Low** | PR-1 creates the file; PR-2 adds 3 lines at known insertion points. No shared logic. |
| `internal/cmux/model.go` | Both PRs | **Low** | PR-1 creates the file; PR-2 adds 3 bool fields at the end of the struct. |
| `internal/cmux/cmux_test.go` | Both PRs | **Low** | PR-1 creates the file; PR-2 appends blocks at the end. Append-only addition. |
| `cmd/brevity/main.go` | Both PRs | **Medium** | Both PRs add to `parseCmuxOptions`, the `cliOptions` struct, and `writeCmuxUsage`. If master receives concurrent additions to these same areas, a rebase will be needed. |
| `cmd/brevity/main_test.go` | PR-1 only | **Low** | PR-1 adds help-output tests; PR-2 does not touch this file. |
| `docs/*` | Both PRs | **Low** | New files only; no existing doc files modified. |
| `internal/cmux/render_*.go` | One PR each | **None** | Each renderer is a new file with no overlap. |

### Concurrent master changes that would require rebase

1. Any commit on master that adds a new CLI flag to `parseCmuxOptions` would conflict with both PRs in `main.go`.
2. Any commit on master that modifies the `RenderOptions` struct fields (adding or reordering) would conflict with `model.go` in PR-2.
3. Any commit on master that adds test coverage for `brevity cmux` in `main_test.go` would conflict with PR-1.

**Mitigations:**
- Merge PR-1 promptly after review to minimise the window where master can drift.
- PR-2 should be opened against master only after PR-1 is merged, not against `feature/cmux-operator-v1`. This keeps the PR-2 diff small and focused on the Phase 2 additions only.

---

## 5. Commit Hygiene

### Duplicate message: `ca9ff6d` and `94bc384`

Both commits carry the message `"Improve CMUX read-only detail rendering"` but touch entirely different files:

| Hash | Files | Lines |
|---|---|---|
| `ca9ff6d` | `cmd/brevity/main_test.go` only | +8, -1 |
| `94bc384` | `internal/cmux/render.go`, `internal/cmux/cmux_test.go` | +531, -24 |

**Assessment:** These are genuinely separate changes that landed 30 seconds apart (00:08:06 and 00:08:36) — likely a split commit that accidentally reused the message.

**Recommended action:** Leave as-is. The constraints for this task prohibit rebasing or squashing, and the duplicate has no functional impact. The PR description for PR-1 can note the split and clarify that both commits are intentional. A future cleanup pass could rename `ca9ff6d` to `"Add CMUX main_test coverage for read-only detail rendering"` if the team values strict message uniqueness, but it is not worth an interactive rebase over a cosmetic issue.

**Do not** squash the entire branch to hide it — that would lose the per-feature commit granularity that makes `git bisect` and `git log --follow` useful on the new render files.

---

## 6. Validation Matrix

### PR-1 validation

| Check | Command | Expected |
|---|---|---|
| Unit tests | `go test ./internal/cmux` | `ok` |
| Integration tests | `go test ./cmd/brevity` | `ok` |
| Text output (default) | `go run ./cmd/brevity cmux` | Header + sections visible |
| Section filter | `go run ./cmd/brevity cmux --section tasks` | Tasks section only |
| State filter | `go run ./cmd/brevity cmux --state reviewing` | Filtered task list |
| Task slug filter | `go run ./cmd/brevity cmux --task <slug>` | Single task or "not found" |
| Limit | `go run ./cmd/brevity cmux --limit 2` | Truncation header if >2 tasks |
| Markdown output | `go run ./cmd/brevity cmux --output markdown` | GFM with `#` headings |
| JSON output | `go run ./cmd/brevity cmux --output json` | Valid JSON, schema `brevity.cmux-report.v1` |
| Review packet (text) | `go run ./cmd/brevity cmux --review <slug>` | Review packet or "not found" |
| Review packet (MD) | `go run ./cmd/brevity cmux --review <slug> --output markdown` | GFM review packet |
| Review packet (JSON) | `go run ./cmd/brevity cmux --review <slug> --output json` | Schema `brevity.cmux-review-report.v1` |
| Help output | `go run ./cmd/brevity cmux --help` | Usage text, all flags listed |
| Whitespace | `git diff --check master` | No whitespace errors |

### PR-2 validation

| Check | Command | Expected |
|---|---|---|
| Unit tests | `go test ./internal/cmux` | `ok` (all 3 new test suites pass) |
| Integration tests | `go test ./cmd/brevity` | `ok` |
| Handoff (text) | `go run ./cmd/brevity cmux --handoff` | Handoff packet, 6 sections, safety note |
| Handoff (MD) | `go run ./cmd/brevity cmux --handoff --output markdown` | GFM, `# CMUX Handoff Packet` |
| Handoff (JSON) | `go run ./cmd/brevity cmux --handoff --output json` | Schema `brevity.cmux-handoff.v1`, `safety.readOnly=true` |
| Merge report (text) | `go run ./cmd/brevity cmux --merge-report` | 6 groups: ready-for-merge … other |
| Merge report (MD) | `go run ./cmd/brevity cmux --merge-report --output markdown` | GFM, `# CMUX Merge Readiness` |
| Merge report (JSON) | `go run ./cmd/brevity cmux --merge-report --output json` | Schema `brevity.cmux-merge-report.v1`, 6-element groups array |
| Blocked report (text) | `go run ./cmd/brevity cmux --blocked-report` | 4 groups, summary line, safety note |
| Blocked report (MD) | `go run ./cmd/brevity cmux --blocked-report --output markdown` | GFM, `# CMUX Blocked Report` |
| Blocked report (JSON) | `go run ./cmd/brevity cmux --blocked-report --output json` | Schema `brevity.cmux-blocked-report.v1`, flat group arrays |
| PR-1 regression | `go run ./cmd/brevity cmux` | Normal report unchanged, no new sections |
| Whitespace | `git diff --check master` | No whitespace errors |

---

## 7. Recommendation

### Final recommendation: **Split into two PRs**

**PR-1** lands Phase 1 (10 commits, ~4,600 lines). It is the complete, standalone CMUX operator report: reader, text/markdown/json output, section/task/state filtering, limit, review packet, full test coverage, and usage documentation. It can be reviewed, validated, and merged independently.

**PR-2** lands Phase 2 (4 commits, ~3,250 lines). It builds on PR-1 without modifying its logic. The three new reports (`--handoff`, `--merge-report`, `--blocked-report`) each live in an isolated `render_*.go` file. The only shared surface is three bool fields on `RenderOptions` and three dispatch guards in `Render()`. PR-2 should be opened against master immediately after PR-1 merges.

### Why not one PR

A single 7,836-line PR covering a new package, three output modes, filtering, and four specialised report formats is too wide for a focused review. The two-phase boundary already exists in the commit history at `857807f` → `d99cc92`, making the split natural rather than artificial.

### Why not three or more PRs

Option C (separate PRs for text vs. markdown+JSON) would produce an intermediate state where the `brevity cmux` command only supports `--output text`. That state is incomplete enough to be confusing on master and adds sequencing overhead with no benefit. The output modes, filtering, and review packet are a coherent Phase 1 unit.

### Merge order

```
1. Open and merge PR-1  (Phase 1 core + review packet)
2. Rebase PR-2 onto updated master
3. Open and merge PR-2  (Phase 2 reports)
4. Delete feature/cmux-operator-v1
```

The rebase in step 2 is expected to be trivial: the PR-2 additions to `parseCmuxOptions` and `RenderOptions` are at well-defined insertion points and are unlikely to conflict with anything PR-1 itself introduced.

---

*Document prepared from `git log --oneline feature/cmux-operator-v1 ^master` and `git diff --stat master...HEAD`. No code was changed; no git operations were performed.*
