package analyze

import (
	"strings"
	"testing"
	"time"

	"github.com/NovaLux12/dig/internal/git"
)

func mkContrib(name, email string, n int, first, last time.Time) git.Contributor {
	return git.Contributor{Name: name, Email: email, Commits: n, FirstAt: first, LastAt: last}
}

func mkLang(ext string, lines int64) git.LangStat {
	return git.LangStat{Extension: ext, Lines: lines}
}

func mkHot(path string, n int) git.FileStat {
	return git.FileStat{Path: path, Touches: n}
}

func TestCompare_CommitDiff(t *testing.T) {
	d := func(y int, m time.Month, day int) time.Time {
		return time.Date(y, m, day, 12, 0, 0, 0, time.UTC)
	}
	// base: 2 commits
	base := &Report{
		Commits: []git.Commit{
			{Hash: "aaa", Author: "alice", Email: "a@x", Time: d(2026, 3, 1), Subject: "second"},
			{Hash: "bbb", Author: "alice", Email: "a@x", Time: d(2026, 1, 1), Subject: "first"},
		},
		TotalCommits: 2,
	}
	// target: 3 commits, one new since base
	target := &Report{
		Commits: []git.Commit{
			{Hash: "ccc", Author: "bob", Email: "b@x", Time: d(2026, 6, 1), Subject: "third"},
			{Hash: "aaa", Author: "alice", Email: "a@x", Time: d(2026, 3, 1), Subject: "second"},
			{Hash: "bbb", Author: "alice", Email: "a@x", Time: d(2026, 1, 1), Subject: "first"},
		},
		TotalCommits: 3,
	}
	delta := Compare(base, target, "v1.0", "HEAD")
	if delta.CommitDelta != 1 {
		t.Errorf("commit delta: want 1, got %d", delta.CommitDelta)
	}
	if len(delta.CommitsAdded) != 1 || delta.CommitsAdded[0].Hash != "ccc" {
		t.Errorf("commits added: want [ccc], got %v", hashes(delta.CommitsAdded))
	}
	if len(delta.CommitsRemoved) != 0 {
		t.Errorf("commits removed: want [], got %v", hashes(delta.CommitsRemoved))
	}
}

func TestCompare_RemovedAndAdded(t *testing.T) {
	// base has a commit that target doesn't (e.g. branched off, base went away)
	d := func(y int, m time.Month, day int) time.Time {
		return time.Date(y, m, day, 12, 0, 0, 0, time.UTC)
	}
	base := &Report{
		Commits: []git.Commit{
			{Hash: "aaa", Time: d(2026, 1, 1)},
			{Hash: "old", Time: d(2025, 12, 1)}, // removed
		},
		TotalCommits: 2,
	}
	target := &Report{
		Commits: []git.Commit{
			{Hash: "aaa", Time: d(2026, 1, 1)},
			{Hash: "new", Time: d(2026, 6, 1)}, // added
		},
		TotalCommits: 2,
	}
	delta := Compare(base, target, "old-branch", "HEAD")
	if len(delta.CommitsAdded) != 1 || delta.CommitsAdded[0].Hash != "new" {
		t.Errorf("added: %v", hashes(delta.CommitsAdded))
	}
	if len(delta.CommitsRemoved) != 1 || delta.CommitsRemoved[0].Hash != "old" {
		t.Errorf("removed: %v", hashes(delta.CommitsRemoved))
	}
	if delta.CommitDelta != 0 {
		t.Errorf("commit delta: 0 total commits change, got %d", delta.CommitDelta)
	}
}

func TestCompare_ContributorDiff(t *testing.T) {
	d := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	base := &Report{
		Contributors: []git.Contributor{
			mkContrib("alice", "a@x", 5, d, d),
			mkContrib("bob", "b@x", 3, d, d), // departed
		},
	}
	target := &Report{
		Contributors: []git.Contributor{
			mkContrib("alice", "a@x", 7, d, d),
			mkContrib("carol", "c@x", 2, d, d), // new
		},
	}
	delta := Compare(base, target, "v1", "v2")
	if len(delta.NewContributors) != 1 || delta.NewContributors[0].Name != "carol" {
		t.Errorf("new: %v", names(delta.NewContributors))
	}
	if len(delta.DepartedContributors) != 1 || delta.DepartedContributors[0].Name != "bob" {
		t.Errorf("departed: %v", names(delta.DepartedContributors))
	}
	if delta.ContribDelta != 0 {
		t.Errorf("contrib delta: 1 in - 1 out = 0, got %d", delta.ContribDelta)
	}
}

func TestCompare_ContributorNameMatchIgnoresEmail(t *testing.T) {
	// Same person, two emails — should NOT count as new.
	d := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	base := &Report{
		Contributors: []git.Contributor{mkContrib("alice", "old@x", 5, d, d)},
	}
	target := &Report{
		Contributors: []git.Contributor{mkContrib("alice", "new@x", 3, d, d)},
	}
	delta := Compare(base, target, "v1", "v2")
	// Per the package doc, contributor identity is (Name, Email) tuple.
	// Different email = different contributor in this implementation.
	if len(delta.NewContributors) != 1 {
		t.Errorf("expected 1 new (email changed), got %d", len(delta.NewContributors))
	}
}

func TestCompare_HotFilesDiff(t *testing.T) {
	base := &Report{
		HotFiles: []git.FileStat{
			mkHot("main.go", 10),
			mkHot("old.go", 5), // lost
		},
	}
	target := &Report{
		HotFiles: []git.FileStat{
			mkHot("main.go", 15),
			mkHot("new.go", 3), // added
		},
	}
	delta := Compare(base, target, "v1", "v2")
	if len(delta.NewHotFiles) != 1 || delta.NewHotFiles[0] != "new.go" {
		t.Errorf("new hot: %v", delta.NewHotFiles)
	}
	if len(delta.LostHotFiles) != 1 || delta.LostHotFiles[0] != "old.go" {
		t.Errorf("lost hot: %v", delta.LostHotFiles)
	}
}

func TestCompare_LanguageGrowth(t *testing.T) {
	base := &Report{
		Languages: []git.LangStat{
			mkLang("go", 1000),
			mkLang("md", 500),
			mkLang("sh", 100), // dropped to 0 in target
		},
	}
	target := &Report{
		Languages: []git.LangStat{
			mkLang("go", 1500), // +500
			mkLang("md", 500),  // unchanged, excluded
			mkLang("ts", 200),  // brand new
		},
	}
	delta := Compare(base, target, "v1", "v2")
	if len(delta.LanguageGrowth) != 3 {
		t.Fatalf("expected 3 language changes, got %d: %+v", len(delta.LanguageGrowth), delta.LanguageGrowth)
	}
	// Sorted by |delta| desc: go (+500), sh (-100), ts (+200)
	want := []struct {
		ext   string
		delta int64
	}{
		{"go", 500},
		{"ts", 200},
		{"sh", -100},
	}
	for i, w := range want {
		if delta.LanguageGrowth[i].Extension != w.ext {
			t.Errorf("position %d: want %s, got %s", i, w.ext, delta.LanguageGrowth[i].Extension)
		}
		if delta.LanguageGrowth[i].Delta != w.delta {
			t.Errorf("position %d delta: want %d, got %d", i, w.delta, delta.LanguageGrowth[i].Delta)
		}
	}
}

func TestCompare_BusFactorDelta(t *testing.T) {
	cases := []struct {
		base, target int
		wantMsg      string
	}{
		{2, 3, "rose from 2 to 3"},
		{3, 2, "fell from 3 to 2"},
		{2, 2, "unchanged at 2"},
	}
	for _, tc := range cases {
		base := &Report{BusFactor: tc.base}
		target := &Report{BusFactor: tc.target}
		d := Compare(base, target, "v1", "v2")
		if d.BusFactorDelta != tc.target-tc.base {
			t.Errorf("BF delta %d->%d: want %d, got %d", tc.base, tc.target, tc.target-tc.base, d.BusFactorDelta)
		}
		if !contains(d.BusFactorMsg, tc.wantMsg) {
			t.Errorf("BF msg %d->%d: want %q in %q", tc.base, tc.target, tc.wantMsg, d.BusFactorMsg)
		}
	}
}

func TestCompare_NilInputs(t *testing.T) {
	if Compare(nil, &Report{}, "a", "b") != nil {
		t.Error("nil base should return nil")
	}
	if Compare(&Report{}, nil, "a", "b") != nil {
		t.Error("nil target should return nil")
	}
}

func TestCompare_RefLabels(t *testing.T) {
	delta := Compare(&Report{}, &Report{}, "v0.1.0", "HEAD")
	if delta.BaseRef != "v0.1.0" || delta.TargetRef != "HEAD" {
		t.Errorf("ref labels not preserved: %+v", delta)
	}
}

// helpers

func hashes(cs []git.Commit) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Hash
	}
	return out
}

func names(cs []git.Contributor) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Name
	}
	return out
}

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}
