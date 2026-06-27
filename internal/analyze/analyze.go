// Package analyze turns the raw commit and file data pulled by internal/git
// into the derived structures that the report needs: timelines, hot files,
// language histograms, and bus factor.
//
// All functions in this package are pure — they take in already-fetched
// git.* values and return analyze.* values. They do not shell out, do not
// touch the filesystem, and have no side effects. This makes the analysis
// trivial to unit-test with hand-constructed inputs.
package analyze

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/NovaLux12/dig/internal/git"
)

// Report is the full data the report renderer needs.
type Report struct {
	RepoName     string
	RepoPath     string
	FirstCommit  git.Commit
	LastCommit   git.Commit
	TotalCommits int
	Commits      []git.Commit // all commits in reverse-chronological order
	Contributors []git.Contributor
	BusFactor    int
	BusFactorMsg string
	Timeline     []MonthBucket
	HotFiles     []git.FileStat
	Languages    []git.LangStat
	FileCount    int
	Readme       string
	GeneratedAt  time.Time
	Accent       string
}

// MonthBucket is one calendar month of activity.
type MonthBucket struct {
	Year    int
	Month   time.Month
	Commits int
	Label   string // "2026-03"
}

// Build runs every analysis pass and returns the populated Report. The inputs
// are the raw git data fetched elsewhere; Build does no I/O of its own.
func Build(
	repoPath, repoName string,
	commits []git.Commit,
	contributors []git.Contributor,
	hotFiles []git.FileStat,
	languages []git.LangStat,
	fileCount int,
	readme string,
	accent string,
) (*Report, error) {
	if len(commits) == 0 {
		return nil, fmt.Errorf("no commits in repository")
	}

	r := &Report{
		RepoName:     repoName,
		RepoPath:     repoPath,
		FirstCommit:  commits[len(commits)-1], // commits are reverse-chrono
		LastCommit:   commits[0],
		TotalCommits: len(commits),
		Commits:      commits,
		Contributors: contributors,
		HotFiles:     hotFiles,
		Languages:    languages,
		FileCount:    fileCount,
		Readme:       readme,
		GeneratedAt:  time.Now().UTC(),
		Accent:       accent,
	}

	r.Timeline = BuildTimeline(commits)
	r.BusFactor, r.BusFactorMsg = BusFactor(contributors)

	return r, nil
}

// BuildTimeline groups commits by calendar month and returns one bucket per
// month from the first commit's month through the last commit's month.
// Months with no commits have Commits=0 (the timeline shows the silence).
func BuildTimeline(commits []git.Commit) []MonthBucket {
	if len(commits) == 0 {
		return nil
	}
	first := commits[len(commits)-1].Time
	last := commits[0].Time
	if first.After(last) {
		first, last = last, first
	}

	startYear, startMonth, _ := first.Date()
	endYear, endMonth, _ := last.Date()

	type key struct {
		Year  int
		Month time.Month
	}
	byKey := map[key]int{}

	for _, c := range commits {
		y, m, _ := c.Time.Date()
		byKey[key{y, m}]++
	}

	var out []MonthBucket
	for y, m := startYear, startMonth; ; y, m = nextMonth(y, m) {
		b := MonthBucket{
			Year:    y,
			Month:   m,
			Commits: byKey[key{y, m}],
			Label:   fmt.Sprintf("%04d-%02d", y, m),
		}
		out = append(out, b)
		if y == endYear && m == endMonth {
			break
		}
	}
	return out
}

// nextMonth returns (year, month) for the month after the given one.
func nextMonth(y int, m time.Month) (int, time.Month) {
	if m == time.December {
		return y + 1, time.January
	}
	return y, m + 1
}

// BusFactor computes the smallest number of contributors whose commit
// share (sorted by count desc) reaches 50% of total commits.
//
// The definition is the standard "remove the smallest set whose remaining
// work exceeds X% of activity." dig uses X = 0.5. If a single contributor
// has 50%+, bus factor is 1 (the project is a one-person bus).
//
// Returns (0, friendly) when there are no contributors.
func BusFactor(cons []git.Contributor) (int, string) {
	if len(cons) == 0 {
		return 0, "no contributors"
	}
	var total int
	for _, c := range cons {
		total += c.Commits
	}
	if total == 0 {
		return 0, "no commits"
	}
	const threshold = 0.5
	var sum int
	for i, c := range cons {
		sum += c.Commits
		if float64(sum)/float64(total) >= threshold {
			bf := i + 1
			msg := fmt.Sprintf(
				"%d contributor%s could disappear before 50%% of commits become unmaintained",
				bf, plural(bf))
			return bf, msg
		}
	}
	// Should be unreachable given threshold <= 1 and at least one commit,
	// but be defensive.
	return len(cons), "all contributors"
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// MonthsSpan returns (months, totalCommits) for a timeline. Used by the
// renderer to decide whether to log-scale the bar chart.
func (r *Report) MonthsSpan() (int, int) {
	if len(r.Timeline) == 0 {
		return 0, 0
	}
	var total int
	for _, b := range r.Timeline {
		total += b.Commits
	}
	return len(r.Timeline), total
}

// PeakMonth returns the (label, commits) of the busiest single month. Returns
// ("", 0) for an empty timeline.
func (r *Report) PeakMonth() (string, int) {
	if len(r.Timeline) == 0 {
		return "", 0
	}
	best := r.Timeline[0]
	for _, b := range r.Timeline[1:] {
		if b.Commits > best.Commits {
			best = b
		}
	}
	return best.Label, best.Commits
}

// Age returns the duration between the first and last commit.
func (r *Report) Age() time.Duration {
	return r.LastCommit.Time.Sub(r.FirstCommit.Time)
}

// TopContributorCommits returns the commit count of the most prolific
// contributor (used to scale contributor share bars).
func (r *Report) TopContributorCommits() int {
	if len(r.Contributors) == 0 {
		return 0
	}
	return r.Contributors[0].Commits
}

// ContributorShare returns the share (0..1) of contributor i relative to the
// top contributor, or 0 if there are no contributors.
func (r *Report) ContributorShare(i int) float64 {
	if i < 0 || i >= len(r.Contributors) {
		return 0
	}
	top := r.TopContributorCommits()
	if top == 0 {
		return 0
	}
	return float64(r.Contributors[i].Commits) / float64(top)
}

// TimelineMax returns the highest per-month commit count, used for scaling
// the SVG bars. Returns 0 for empty timelines.
func (r *Report) TimelineMax() int {
	if len(r.Timeline) == 0 {
		return 0
	}
	m := 0
	for _, b := range r.Timeline {
		if b.Commits > m {
			m = b.Commits
		}
	}
	return m
}

// TimelineBarHeight returns the SVG height (in pixels) for a bar of the
// given commit count, given the chart's total drawing height. We use a
// log scale so a single mega-month doesn't flatten everything else to
// invisible. The mapping is: h = H * log(1+c) / log(1+max).
func (r *Report) TimelineBarHeight(c int, chartHeight int) int {
	max := r.TimelineMax()
	if max == 0 || chartHeight <= 0 {
		return 0
	}
	if c <= 0 {
		return 1 // floor at 1px so empty months still render a thin line
	}
	ratio := math.Log(1+float64(c)) / math.Log(1+float64(max))
	return int(math.Round(float64(chartHeight) * ratio))
}

// HotFileTops returns at most n entries, each carrying enough info for the
// renderer's table.
func (r *Report) HotFileTops(n int) []git.FileStat {
	if n <= 0 || len(r.HotFiles) <= n {
		return r.HotFiles
	}
	out := make([]git.FileStat, n)
	copy(out, r.HotFiles[:n])
	return out
}

// ContributorTops is like HotFileTops but for contributors.
func (r *Report) ContributorTops(n int) []git.Contributor {
	if n <= 0 || len(r.Contributors) <= n {
		return r.Contributors
	}
	out := make([]git.Contributor, n)
	copy(out, r.Contributors[:n])
	return out
}

// LanguageTops is like HotFileTops but for languages.
func (r *Report) LanguageTops(n int) []git.LangStat {
	if n <= 0 || len(r.Languages) <= n {
		return r.Languages
	}
	out := make([]git.LangStat, n)
	copy(out, r.Languages[:n])
	return out
}

// SortedLanguages returns languages sorted by lines desc. Kept as a method
// for callers that need a stable ordering even if r.Languages was mutated.
func (r *Report) SortedLanguages() []git.LangStat {
	out := make([]git.LangStat, len(r.Languages))
	copy(out, r.Languages)
	sort.Slice(out, func(i, j int) bool { return out[i].Lines > out[j].Lines })
	return out
}
