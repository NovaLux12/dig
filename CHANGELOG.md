# Changelog

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
