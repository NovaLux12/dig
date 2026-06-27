// Package analyze — compare.go
//
// Compare takes two Reports and produces a Delta describing how the target
// differs from the base. Both Reports must be built from non-empty repos;
// Compare itself does no I/O.
//
// Identity matching:
//   - commits: SHA hash
//   - contributors: (display name, email) tuple
//   - hot files: path string
//   - languages: file extension
package analyze

import (
	"fmt"
	"sort"

	"github.com/NovaLux12/dig/internal/git"
)

// Delta describes how a target Report differs from a base Report. Both
// Reports are kept on the Delta so the renderer can show the labels and
// the underlying totals (e.g. "12 commits since v1.0").
type Delta struct {
	BaseRef   string // display label, e.g. "v1.0", "main", "abc1234"
	TargetRef string // display label, e.g. "HEAD"

	CommitDelta    int // target.TotalCommits - base.TotalCommits
	ContribDelta   int // len(NewContributors) - len(DepartedContributors)
	BusFactorDelta int
	BusFactorMsg   string // human-readable summary of bus-factor change

	CommitsAdded         []git.Commit // in target, not in base (most recent first)
	CommitsRemoved       []git.Commit // in base, not in target (oldest first)
	NewContributors      []git.Contributor
	DepartedContributors []git.Contributor
	NewHotFiles          []string // paths only in target's top-N
	LostHotFiles         []string // paths only in base's top-N
	LanguageGrowth       []LanguageChange
}

// LanguageChange reports how an extension's line count shifted.
type LanguageChange struct {
	Extension   string
	BaseLines   int64
	TargetLines int64
	Delta       int64 // signed
}

// Compare returns the Delta between base and target. Both must be non-nil.
//
// baseRef / targetRef are display labels shown in the rendered report.
// They don't affect the diff — they're metadata for the human reading
// the HTML.
func Compare(base, target *Report, baseRef, targetRef string) *Delta {
	if base == nil || target == nil {
		return nil
	}

	d := &Delta{
		BaseRef:        baseRef,
		TargetRef:      targetRef,
		CommitDelta:    target.TotalCommits - base.TotalCommits,
		BusFactorDelta: target.BusFactor - base.BusFactor,
	}

	d.CommitsAdded, d.CommitsRemoved = diffCommits(base.Commits, target.Commits)
	d.NewContributors, d.DepartedContributors = diffContributors(base.Contributors, target.Contributors)
	d.ContribDelta = len(d.NewContributors) - len(d.DepartedContributors)
	d.NewHotFiles, d.LostHotFiles = diffHotFiles(base.HotFiles, target.HotFiles)
	d.LanguageGrowth = diffLanguages(base.Languages, target.Languages)

	d.BusFactorMsg = busFactorDeltaMsg(base.BusFactor, target.BusFactor)

	return d
}

// diffCommits returns (inTargetNotInBase, inBaseNotInTarget). Both slices
// are sorted by time desc (most recent first) for stable display.
func diffCommits(base, target []git.Commit) (added, removed []git.Commit) {
	baseSet := make(map[string]bool, len(base))
	for _, c := range base {
		baseSet[c.Hash] = true
	}
	targetSet := make(map[string]bool, len(target))
	for _, c := range target {
		targetSet[c.Hash] = true
	}
	for _, c := range target {
		if !baseSet[c.Hash] {
			added = append(added, c)
		}
	}
	for _, c := range base {
		if !targetSet[c.Hash] {
			removed = append(removed, c)
		}
	}
	// Most recent first; tie-break by hash for determinism.
	sort.Slice(added, func(i, j int) bool {
		if !added[i].Time.Equal(added[j].Time) {
			return added[i].Time.After(added[j].Time)
		}
		return added[i].Hash < added[j].Hash
	})
	sort.Slice(removed, func(i, j int) bool {
		if !removed[i].Time.Equal(removed[j].Time) {
			return removed[i].Time.After(removed[j].Time)
		}
		return removed[i].Hash < removed[j].Hash
	})
	return
}

// diffContributors matches by (Name, Email) tuple. Both slices are sorted
// by commit count desc for stable display.
func diffContributors(base, target []git.Contributor) (newC, departed []git.Contributor) {
	key := func(c git.Contributor) string { return c.Name + " <" + c.Email + ">" }
	baseSet := make(map[string]bool, len(base))
	for _, c := range base {
		baseSet[key(c)] = true
	}
	targetSet := make(map[string]bool, len(target))
	for _, c := range target {
		targetSet[key(c)] = true
	}
	for _, c := range target {
		if !baseSet[key(c)] {
			newC = append(newC, c)
		}
	}
	for _, c := range base {
		if !targetSet[key(c)] {
			departed = append(departed, c)
		}
	}
	sort.Slice(newC, func(i, j int) bool { return newC[i].Commits > newC[j].Commits })
	sort.Slice(departed, func(i, j int) bool { return departed[i].Commits > departed[j].Commits })
	return
}

// diffHotFiles returns (onlyInTarget, onlyInBase) by path.
func diffHotFiles(base, target []git.FileStat) (newF, lost []string) {
	baseSet := make(map[string]bool, len(base))
	for _, f := range base {
		baseSet[f.Path] = true
	}
	for _, f := range target {
		if !baseSet[f.Path] {
			newF = append(newF, f.Path)
		}
	}
	targetSet := make(map[string]bool, len(target))
	for _, f := range target {
		targetSet[f.Path] = true
	}
	for _, f := range base {
		if !targetSet[f.Path] {
			lost = append(lost, f.Path)
		}
	}
	return
}

// diffLanguages returns one LanguageChange per extension that shifted in
// either direction. Sorted by absolute delta desc (biggest movers first).
func diffLanguages(base, target []git.LangStat) []LanguageChange {
	baseBy := make(map[string]int64, len(base))
	for _, l := range base {
		baseBy[l.Extension] = l.Lines
	}
	seen := make(map[string]bool, len(target))
	var out []LanguageChange
	for _, l := range target {
		seen[l.Extension] = true
		b := baseBy[l.Extension]
		if l.Lines == b {
			continue
		}
		out = append(out, LanguageChange{
			Extension:   l.Extension,
			BaseLines:   b,
			TargetLines: l.Lines,
			Delta:       l.Lines - b,
		})
	}
	for _, l := range base {
		if seen[l.Extension] {
			continue
		}
		if l.Lines == 0 {
			continue
		}
		out = append(out, LanguageChange{
			Extension:   l.Extension,
			BaseLines:   l.Lines,
			TargetLines: 0,
			Delta:       -l.Lines,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		ai := out[i].Delta
		if ai < 0 {
			ai = -ai
		}
		aj := out[j].Delta
		if aj < 0 {
			aj = -aj
		}
		return ai > aj
	})
	return out
}

// busFactorDeltaMsg returns a one-line summary of the bus-factor shift.
func busFactorDeltaMsg(baseBF, targetBF int) string {
	switch {
	case targetBF > baseBF:
		return fmt.Sprintf("bus factor rose from %d to %d (more contributors needed for half of commits)", baseBF, targetBF)
	case targetBF < baseBF:
		return fmt.Sprintf("bus factor fell from %d to %d (fewer contributors needed for half of commits)", baseBF, targetBF)
	default:
		return fmt.Sprintf("bus factor unchanged at %d", targetBF)
	}
}
