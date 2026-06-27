package report

// reportHTML is the page template. It is kept as a Go string rather than a
// separate file so the binary stays single-source-file (no embed needed for
// a 7 KB template). Everything visible — CSS, SVG, layout — is here.
//
// The template uses html/template's safe-by-default escaping. Fields marked
// `template.HTML` (the SVG and bar HTML) are pre-sanitised by the renderer.
const reportHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.RepoName}} — dig report</title>
<style>
:root {
  --accent: {{.Accent}};
  --accent-hover: color-mix(in srgb, var(--accent) 70%, white 30%);
  --bg: #0e1116;
  --surface: #181b22;
  --fg: #cdd6f4;
  --muted: #6c7086;
  --border: #2a2f3a;
}
@media (prefers-color-scheme: light) {
  :root {
    --bg: #fafafa;
    --surface: #ffffff;
    --fg: #1f2328;
    --muted: #6e7781;
    --border: #d0d7de;
  }
}
* { box-sizing: border-box; }
html, body { margin: 0; padding: 0; background: var(--bg); color: var(--fg); }
body {
  font: 15px/1.55 -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif;
  padding: 32px 24px 64px;
}
.mono { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
.container { max-width: 960px; margin: 0 auto; }
header.page {
  margin-bottom: 32px;
  padding-bottom: 20px;
  border-bottom: 1px solid var(--border);
}
h1 { font-size: 26px; margin: 0 0 4px; letter-spacing: -0.01em; }
.subtitle { color: var(--muted); font-size: 14px; }
.subtitle code { color: var(--fg); }

section.card {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 12px;
  padding: 20px;
  margin-bottom: 20px;
}
section.card h2 {
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--muted);
  margin: 0 0 16px;
  font-weight: 600;
}

.metrics {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 12px;
}
.metric {
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 12px 14px;
}
.metric .label { font-size: 11px; color: var(--muted); text-transform: uppercase; letter-spacing: 0.06em; }
.metric .value { font-size: 20px; font-weight: 600; margin-top: 4px; }
.metric .value.accent { color: var(--accent); }

.busfactor-callout {
  margin-top: 12px;
  padding: 12px 14px;
  border-left: 3px solid var(--accent);
  background: color-mix(in srgb, var(--accent) 8%, transparent);
  border-radius: 0 6px 6px 0;
  font-size: 14px;
}

.contrib-row {
  display: grid;
  grid-template-columns: minmax(120px, 200px) 1fr 60px;
  align-items: center;
  gap: 12px;
  padding: 4px 0;
}
.contrib-name { font-size: 13px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.contrib-bar {
  height: 6px;
  background: var(--border);
  border-radius: 3px;
  overflow: hidden;
}
.contrib-bar-fill { height: 100%; background: var(--accent); border-radius: 3px; }
.contrib-count { text-align: right; font: 12px ui-monospace, monospace; color: var(--muted); }

table.hot {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}
table.hot th, table.hot td {
  text-align: left;
  padding: 6px 8px;
  border-bottom: 1px solid var(--border);
}
table.hot th {
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--muted);
  font-weight: 600;
}
table.hot td.num, table.hot th.num { text-align: right; font: 12px ui-monospace, monospace; }
table.hot td.path { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; }
table.hot tr:last-child td { border-bottom: 0; }

.lang-row {
  display: grid;
  grid-template-columns: 80px 1fr 200px;
  align-items: center;
  gap: 12px;
  padding: 3px 0;
}
.lang-ext { font: 12px ui-monospace, monospace; color: var(--muted); }
.lang-bar { height: 6px; background: var(--border); border-radius: 3px; overflow: hidden; }
.lang-bar-fill { height: 100%; background: var(--accent); border-radius: 3px; }
.lang-stats { font: 11px ui-monospace, monospace; color: var(--muted); text-align: right; }

.commit-card {
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 14px;
  margin-bottom: 12px;
  background: var(--bg);
}
.commit-card .hash {
  font: 12px ui-monospace, monospace;
  color: var(--accent);
  display: inline-block;
}
.commit-card .subject { font-weight: 600; margin: 4px 0; }
.commit-card .meta { font-size: 12px; color: var(--muted); }
.commit-card .files {
  margin-top: 8px;
  font: 11px ui-monospace, monospace;
  color: var(--muted);
  word-break: break-all;
}

.readme {
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 16px;
  font: 13px/1.6 ui-monospace, SFMono-Regular, Menlo, monospace;
  white-space: pre-wrap;
  overflow-x: auto;
  max-height: 480px;
  overflow-y: auto;
}

footer.page {
  margin-top: 40px;
  padding-top: 20px;
  border-top: 1px solid var(--border);
  font-size: 12px;
  color: var(--muted);
  text-align: center;
}
footer.page a { color: var(--accent); text-decoration: none; }
footer.page a:hover { text-decoration: underline; }
</style>
</head>
<body>
<div class="container">

<header class="page">
  <h1>{{.RepoName}}</h1>
  <div class="subtitle mono">{{.RepoPath}}</div>
</header>

<section class="card">
  <h2>Overview</h2>
  <div class="metrics">
    <div class="metric"><div class="label">Commits</div><div class="value">{{comma .TotalCommits}}</div></div>
    <div class="metric"><div class="label">Contributors</div><div class="value">{{comma .ContributorCount}}</div></div>
    <div class="metric"><div class="label">Age</div><div class="value">{{.Age}}</div></div>
    <div class="metric"><div class="label">Files at HEAD</div><div class="value">{{comma .FileCount}}</div></div>
    <div class="metric"><div class="label">First commit</div><div class="value mono">{{.FirstAt}}</div></div>
    <div class="metric"><div class="label">Last commit</div><div class="value mono">{{.LastAt}}</div></div>
  </div>
  <div class="busfactor-callout">
    <strong>Bus factor:</strong> {{.BusFactorMsg}}.
  </div>
</section>

<section class="card">
  <h2>Timeline</h2>
  <div>{{.TimelineSVG}}</div>
  <div class="subtitle" style="margin-top:8px">
    Peak: <span class="mono">{{.PeakLabel}}</span> with {{.PeakCommits}} commits ·
    {{.Months}} months across {{comma .TimelineTotal}} commits
  </div>
</section>

<section class="card">
  <h2>Contributors</h2>
  {{.ContribBars}}
</section>

{{if .HotFiles}}
<section class="card">
  <h2>Hot files</h2>
  <table class="hot">
    <thead>
      <tr><th>Path</th><th>Primary author</th><th class="num">Touches</th><th class="num">Last modified</th></tr>
    </thead>
    <tbody>
      {{range .HotFiles}}
      <tr>
        <td class="path">{{.Path}}</td>
        <td>{{.PrimaryAuthor}}</td>
        <td class="num">{{comma .Touches}}</td>
        <td class="num">{{.LastModified.Format "2006-01-02"}}</td>
      </tr>
      {{end}}
    </tbody>
  </table>
</section>
{{end}}

{{if .Languages}}
<section class="card">
  <h2>Languages</h2>
  {{.LangBars}}
</section>
{{end}}

<section class="card">
  <h2>First commit</h2>
  <div class="commit-card">
    <div class="hash">{{shortHash .FirstCommit.Hash}}</div>
    <div class="subject">{{.FirstCommit.Subject}}</div>
    <div class="meta">{{.FirstCommit.Author}} &lt;{{.FirstCommit.Email}}&gt; · {{.FirstCommit.Time.Format "2006-01-02 15:04 MST"}}</div>
    {{if .FirstCommit.Files}}
    <div class="files">
      {{range .FirstCommit.Files}}{{.Path}}
      {{end}}
    </div>
    {{end}}
  </div>
</section>

<section class="card">
  <h2>Latest commit</h2>
  <div class="commit-card">
    <div class="hash">{{shortHash .LastCommit.Hash}}</div>
    <div class="subject">{{.LastCommit.Subject}}</div>
    <div class="meta">{{.LastCommit.Author}} &lt;{{.LastCommit.Email}}&gt; · {{.LastCommit.Time.Format "2006-01-02 15:04 MST"}}</div>
    {{if .LastCommit.Files}}
    <div class="files">
      {{range .LastCommit.Files}}{{.Path}}
      {{end}}
    </div>
    {{end}}
  </div>
</section>

{{if .Readme}}
<section class="card">
  <h2>README excerpt</h2>
  <div class="readme">{{.Readme}}</div>
</section>
{{end}}

<footer class="page">
  Generated by <a href="https://github.com/NovaLux12/dig">dig</a> on {{.GeneratedAt}}
</footer>

</div>
</body>
</html>
`
