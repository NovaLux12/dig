// Package report renders an analyze.Report into a self-contained HTML
// document. The output is one file: no external CSS, JS, fonts, images,
// or network requests. The visual style is dark by default and respects
// prefers-color-scheme for a light fallback.
package report

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/NovaLux12/dig/internal/analyze"
	"github.com/NovaLux12/dig/internal/git"
)

// Render returns the HTML document as bytes.
func Render(r *analyze.Report) ([]byte, error) {
	tmpl, err := template.New("report").Funcs(funcMap).Parse(reportHTML)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, viewModel(r)); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}
	return buf.Bytes(), nil
}

// viewModel pre-renders the strings the template needs so the template
// itself stays declarative.
func viewModel(r *analyze.Report) map[string]any {
	repoName := r.RepoName
	if repoName == "" {
		repoName = "untitled"
	}

	months, total := r.MonthsSpan()
	age := r.Age()

	share := make([]float64, len(r.Contributors))
	for i := range r.Contributors {
		share[i] = r.ContributorShare(i)
	}

	timelineSVG := renderTimelineSVG(r)
	contribBars := renderContributorBars(r)
	langBars := renderLanguageBars(r)

	// The bus-factor sentence for the header.
	bfMsg := r.BusFactorMsg
	if r.BusFactor == 1 {
		bfMsg = "1 contributor could disappear before 50% of commits become unmaintained"
	}

	return map[string]any{
		"RepoName":         repoName,
		"RepoPath":         r.RepoPath,
		"Accent":           r.Accent,
		"GeneratedAt":      r.GeneratedAt.Format(time.RFC3339),
		"TotalCommits":     r.TotalCommits,
		"Months":           months,
		"Age":              humanDuration(age),
		"FirstAt":          r.FirstCommit.Time.Format("2006-01-02"),
		"LastAt":           r.LastCommit.Time.Format("2006-01-02"),
		"ContributorCount": len(r.Contributors),
		"FileCount":        r.FileCount,
		"Contributors":     r.Contributors,
		"Shares":           share,
		"ContribBars":      template.HTML(contribBars),
		"HotFiles":         r.HotFiles,
		"Languages":        r.Languages,
		"LangBars":         template.HTML(langBars),
		"FirstCommit":      r.FirstCommit,
		"LastCommit":       r.LastCommit,
		"Readme":           r.Readme,
		"BusFactor":        r.BusFactor,
		"BusFactorMsg":     bfMsg,
		"TimelineSVG":      template.HTML(timelineSVG),
		"PeakLabel":        peakLabel(r),
		"PeakCommits":      peakCommits(r),
		"TimelineTotal":    total,
	}
}

func peakLabel(r *analyze.Report) string {
	lbl, _ := r.PeakMonth()
	return lbl
}

func peakCommits(r *analyze.Report) int {
	_, n := r.PeakMonth()
	return n
}

func humanDuration(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	days := int(d / (24 * time.Hour))
	if days < 60 {
		return fmt.Sprintf("%d days", days)
	}
	years := days / 365
	remDays := days % 365
	months := remDays / 30
	if years > 0 {
		return fmt.Sprintf("%d years, %d months", years, months)
	}
	return fmt.Sprintf("%d months", months)
}

var funcMap = template.FuncMap{
	"pct": func(f float64) string { return fmt.Sprintf("%.1f%%", f*100) },
	"shortHash": func(s string) string {
		if len(s) >= 7 {
			return s[:7]
		}
		return s
	},
	"comma":   func(n int) string { return commaInt(int64(n)) },
	"comma64": func(n int64) string { return commaInt(n) },
}

// commaInt formats an int64 with comma thousands separators.
func commaInt(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		if neg {
			return "-" + s
		}
		return s
	}
	var b strings.Builder
	rem := len(s) % 3
	if rem > 0 {
		b.WriteString(s[:rem])
		if len(s) > rem {
			b.WriteByte(',')
		}
	}
	for i := rem; i < len(s); i += 3 {
		b.WriteString(s[i : i+3])
		if i+3 < len(s) {
			b.WriteByte(',')
		}
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// renderTimelineSVG emits a self-contained SVG bar chart of monthly commits.
func renderTimelineSVG(r *analyze.Report) string {
	const (
		barW      = 8
		gap       = 2
		chartH    = 80
		padTop    = 8
		padBottom = 16
		padLeft   = 28
		padRight  = 8
	)
	months := len(r.Timeline)
	if months == 0 {
		return ""
	}
	w := padLeft + padRight + months*(barW+gap)
	h := chartH + padTop + padBottom

	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="0 0 %d %d" width="%d" height="%d" xmlns="http://www.w3.org/2000/svg" role="img" aria-label="Commits per month">`,
		w, h, w, h)
	b.WriteString(`<style>`)
	fmt.Fprintf(&b, `.tl-bar{fill:var(--accent)}.tl-bar:hover{fill:var(--accent-hover)}.tl-axis{stroke:var(--border)}.tl-label{fill:var(--muted);font:10px ui-monospace,monospace}.tl-zero{fill:var(--border);font:10px ui-monospace,monospace}</style>`)

	// Axis baseline.
	fmt.Fprintf(&b, `<line class="tl-axis" x1="%d" y1="%d" x2="%d" y2="%d"/>`,
		padLeft, padTop+chartH, w-padRight, padTop+chartH)

	// Y-axis labels (top and bottom).
	fmt.Fprintf(&b, `<text class="tl-label" x="%d" y="%d" text-anchor="end">%d</text>`,
		padLeft-4, padTop+8, r.TimelineMax())
	fmt.Fprintf(&b, `<text class="tl-label" x="%d" y="%d" text-anchor="end">0</text>`,
		padLeft-4, padTop+chartH+2)

	// Bars.
	for i, m := range r.Timeline {
		x := padLeft + i*(barW+gap)
		bh := r.TimelineBarHeight(m.Commits, chartH)
		y := padTop + (chartH - bh)
		if bh == 0 {
			// Render zero months as a thin marker so the timeline reads
			// as continuous rather than gappy.
			fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="1" class="tl-axis"/>`,
				x, padTop+chartH-1, barW)
			continue
		}
		fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="%d" class="tl-bar" rx="2"><title>%s — %d commits</title></rect>`,
			x, y, barW, bh, m.Label, m.Commits)
	}

	// X-axis labels: first, peak, last month.
	peakIdx := 0
	peakCommits := 0
	for i, m := range r.Timeline {
		if m.Commits > peakCommits {
			peakCommits = m.Commits
			peakIdx = i
		}
	}
	labelAt := func(i int, anchor string) {
		if i < 0 || i >= len(r.Timeline) {
			return
		}
		x := padLeft + i*(barW+gap) + barW/2
		fmt.Fprintf(&b, `<text class="tl-label" x="%d" y="%d" text-anchor="%s">%s</text>`,
			x, padTop+chartH+12, anchor, r.Timeline[i].Label)
	}
	labelAt(0, "start")
	labelAt(peakIdx, "middle")
	labelAt(len(r.Timeline)-1, "end")

	b.WriteString(`</svg>`)
	return b.String()
}

// renderContributorBars returns the inner HTML for the contributor share
// bars — a stack of inline-block divs with widths set to share%.
func renderContributorBars(r *analyze.Report) string {
	var b strings.Builder
	for i, c := range r.Contributors {
		share := r.ContributorShare(i)
		fmt.Fprintf(&b,
			`<div class="contrib-row"><div class="contrib-name">%s</div><div class="contrib-bar"><div class="contrib-bar-fill" style="width:%.2f%%"></div></div><div class="contrib-count">%s</div></div>`,
			template.HTMLEscapeString(c.Name),
			share*100,
			commaInt(int64(c.Commits)),
		)
	}
	return b.String()
}

// renderLanguageBars returns the inner HTML for the languages histogram.
func renderLanguageBars(r *analyze.Report) string {
	if len(r.Languages) == 0 {
		return ""
	}
	max := r.Languages[0].Lines
	if max == 0 {
		max = 1
	}
	var b strings.Builder
	for _, l := range r.Languages {
		share := float64(l.Lines) / float64(max) * 100
		if l.Lines == 0 {
			share = 0.5 // floor so empty buckets still show a thin line
		}
		fmt.Fprintf(&b,
			`<div class="lang-row"><div class="lang-ext">%s</div><div class="lang-bar"><div class="lang-bar-fill" style="width:%.2f%%"></div></div><div class="lang-stats">%s files · %s lines</div></div>`,
			template.HTMLEscapeString(l.Extension),
			share,
			commaInt(int64(l.Files)),
			commaInt(l.Lines),
		)
	}
	return b.String()
}

// ensure git is referenced (it's used via the viewModel types).
var _ = git.Commit{}
