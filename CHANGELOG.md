# Changelog

## v0.3.2 — 2026-08-27

### Bug fix

- **Release binaries now report their version.** `main.go` declares a
  `version` variable that is meant to be overridden via `-ldflags` at
  release time, but the release workflow never passed any `-ldflags`,
  so every released binary printed `dig dev` for `--version`. The
  release build now injects the tag name (`-X main.version=<tag>`) and
  strips symbol tables (`-s -w`) for smaller binaries. Released
  binaries additionally pick up the `docs: advertise pre-built release
  binaries` README change that landed after v0.3.1.

### Documentation

- README usage block now lists the `--all` and `--version` flags,
  which `--help` already advertised.
- Corrected the minimum Go version in the README (1.22, matching
  `go.mod`) — the previous text still said 1.26 after the go.mod
  alignment in v0.3.1.

## v0.3.1 — 2026-08-22

### Maintenance

- README badges (CI, release, Go version, licence) and `go.mod`
  aligned back to the fleet Go 1.22 convention (PR #2). The CI and
  release workflows stay on Go 1.26, matching the current toolchain.
- Install section now advertises the pre-built release binaries.

## v0.3.0 — 2026-08-20

### Feature

- **`--json <file>` and `--top <n>` flags.** `--json` writes a
  machine-readable JSON report alongside the HTML; `--top <n>` limits
  the hot-files table displayed in the HTML (default 25, `0` = all).
  The JSON always contains the full untruncated data, so
  `dig --json report.json --top 5 <repo-path>` gives a clipped HTML
  view and a complete JSON export.

### Fix

- gofmt cleanup in `internal/report/json.go` (CI formatting check).

No third-party dependencies added. Stdlib only.

```
$ dig --json report.json --top 10 ../my-repo
wrote report.html (xx,xxx bytes)
wrote report.json (x,xxx bytes)
```

## v0.2.0 — 2026-06-27

### Feature

- **`--base <ref>` for compare mode.** Walks the commit log for an
  arbitrary ref (branch, tag, or SHA prefix) and emits a delta report
  alongside the target report. The HTML output adds a "Changes since
  &lt;ref&gt;" section with: commits added / removed (with the most
  recent added commits shown as commit cards), new and departed
  contributors, hot files only in one side or the other, language line
  deltas sorted by magnitude, and the bus-factor shift. The base ref's
  data is computed in the same pass as the target data (same `--since`,
  same `--accent`); output filename and other output knobs are
  unchanged.

### Bug fix

- **`git log` parser glued file block to next commit's hash.** The
  existing format `--format=...%ae%x00` did not emit a NUL between the
  commit's email and the file block. Combined with no separator between
  the file block and the next commit's hash, this caused every commit
  after the first in a multi-commit log to be misaligned — its Subject
  field contained the previous commit's file status line. The bug was
  silent because the existing tests only checked the first commit.
  Fixed by emitting both leading and trailing `%x00` in the format
  string, putting a NUL boundary on both sides of the file block.
  Regression test added in `git_test.go` (`TestCommits_RoundTrip` now
  asserts all three fixture subjects).

### Implementation

- New package `analyze.Compare(base, target *Report, baseRef, targetRef
  string) *Delta`. Pure function, hand-constructed-Report tests.
- New flag on `git.Commits`: `CommitOpts.Ref` walks a specific ref
  instead of HEAD or `--all`.
- New field on `analyze.Report`: `Commits []git.Commit` (so `Compare`
  has the raw commit list without re-walking git).
- `report.Render` signature changed: takes an optional `*Delta` as a
  second arg (`nil` for the old single-ref behaviour).
- `report.Render` template extended with a `Changes since` section
  that only renders when a Delta is provided.

No third-party dependencies added. Stdlib only.

```
$ dig --base v1.0 --out since-v1.html ../my-repo
wrote since-v1.html (44,xxx bytes)
```

## v0.1.0 — 2026-06-27

First release. `dig <repo-path>` produces a self-contained HTML
code-archaeology report covering:

- Project header (commits, contributors, age, dominant language)
- Per-month timeline
- Contributors table with share bars
- Bus factor (greedy removal)
- Hot files (top 25)
- Languages histogram
- First and latest commit cards
- README excerpt

Single static Go binary, stdlib only. No third-party dependencies. The
output HTML file has all CSS and SVG embedded — no CDN, no JS framework,
no network required to view.

```
$ go install github.com/NovaLux12/dig@latest
$ dig ../your-repo --out report.html
$ open report.html
```
## Releases

- **v0.1.0** — `https://github.com/NovaLux12/dig/releases/tag/v0.1.0` —
  cross-platform binaries (linux/darwin/windows, amd64/arm64) plus
  SHA256SUMS. Source install: `go install github.com/NovaLux12/dig@v0.1.0`.
