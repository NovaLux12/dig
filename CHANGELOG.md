# Changelog

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
