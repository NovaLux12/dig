# `dig` — design spec

## What it is

A single static Go binary. `dig <repo-path>` produces a single self-contained
HTML file describing the git history of that repo. No server, no JS framework,
no CDN, no network. The output file works offline and embeds everything it needs.

## CLI

```
dig <repo-path>                 # writes dig-report.html in CWD
dig <repo-path> --out report.html
dig <repo-path> --accent #ff5577 # custom accent colour (default #7aa2f7)
dig <repo-path> --since 12mo    # restrict analysis window (default: full history)
dig --version
dig --help
```

Exit codes: 0 success, 1 not a git repo, 2 git not installed, 3 other I/O.
Writes nothing to the repo itself. Read-only.

## Pipeline

```
os/exec("git", ...)  ──►  internal/git    (stateless wrappers, no caching across calls)
                            │
                            ▼
                        internal/analyze   (pure functions, returns a *Report)
                            │
                            ▼
                        internal/report    (HTML template, returns []byte)
```

`internal/git` calls git once per piece of data it needs. No libgit2, no go-git.
The git binary is the source of truth for diff parsing, rename detection, etc.

## Report contents

1. **Header** — repo name (from remote or dir basename), age since first commit,
   total commits, distinct contributors, distinct files in HEAD, dominant language.
2. **Timeline** — SVG bar chart of commits per calendar month over the repo's
   lifetime. Annotations: first commit, last commit, peak month.
3. **Contributors** — table sorted by commit count desc, with a horizontal bar
   showing share. Columns: name, email, commits, share%, first, last.
4. **Bus factor** — greedy removal: minimum number of contributors whose
   removal would orphan >50% of commits in the analysis window. Shown as "X
   contributors could disappear before 50% of recent work becomes unmaintained."
5. **Hot files** — top 25 files by distinct-commit touches across all refs.
   Each row: path, touches, last modified, primary author (most touches).
6. **Languages** — file-extension histogram of HEAD, with line counts via
   `git ls-files | xargs wc -l` (best-effort, skipped on permission errors).
7. **First commit** — full hash, date, author, message, and a `git show
   --stat` of that commit so the reader sees the day-one shape of the project.
8. **Latest commit on HEAD** — same as above, for "where we are now."
9. **README excerpt** — first 80 non-empty lines of README/README.md if present.

All sections except the dynamic ones are wrapped in a self-contained HTML
document with embedded CSS and SVG.

## Visual style

Dark theme by default (light theme respects `prefers-color-scheme: light`
for accessibility). Single accent colour. System font stack for prose,
`ui-monospace, SFMono-Regular, Menlo, monospace` for code/paths.
SVG charts only — no canvas, no Chart.js. Layout via CSS grid; no JS framework.

## Non-goals

- No web service mode.
- No commit-graph visualisation (DAG render).
- No blame annotations in the output.
- No support for non-git VCS.
- No streaming output (the report is rendered in one shot — repos over ~50k
  commits may be slow; we don't optimise that).

## Test strategy

- `internal/git`: tested via `go test` against a `testdata/` repo that is
  initialised in `TestMain`. Pure data-fixture tests, no shelling out to
  user repos.
- `internal/analyze`: tested with hand-constructed `git.Commit` slices.
- `internal/report`: golden-file test with a tiny fixture repo. The golden
  file is checked in. A non-deterministic element (timestamp) is normalised
  before comparison.

## CI

GitHub Actions on push/PR: `go build`, `go test ./...`, `gofmt -l`.
`release.yml` builds cross-platform binaries on tag push and attaches them
to the GitHub release. Not yet wired up in v0.1.0.