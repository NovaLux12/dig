package analyze

import (
	"testing"
	"time"

	"github.com/NovaLux12/dig/internal/git"
)

func mkCommit(t time.Time, author, email, subj string) git.Commit {
	return git.Commit{
		Hash:    "0000000000000000000000000000000000000000",
		Author:  author,
		Email:   email,
		Time:    t,
		Subject: subj,
	}
}

func TestBuildTimeline(t *testing.T) {
	d := func(y int, m time.Month, day int) time.Time {
		return time.Date(y, m, day, 12, 0, 0, 0, time.UTC)
	}
	commits := []git.Commit{
		mkCommit(d(2026, 6, 15), "a", "a@x", "third"),
		mkCommit(d(2026, 3, 1), "b", "b@x", "second"),
		mkCommit(d(2026, 1, 10), "a", "a@x", "first"),
	}
	tl := BuildTimeline(commits)
	// First month is Jan 2026, last is June 2026.
	if tl[0].Label != "2026-01" {
		t.Errorf("first month: %q", tl[0].Label)
	}
	if tl[len(tl)-1].Label != "2026-06" {
		t.Errorf("last month: %q", tl[len(tl)-1].Label)
	}
	// 2026-01, 02, 03, 04, 05, 06 — six buckets.
	if len(tl) != 6 {
		t.Errorf("want 6 months, got %d", len(tl))
	}
	if tl[0].Commits != 1 {
		t.Errorf("jan commits: %d", tl[0].Commits)
	}
	if tl[2].Commits != 1 {
		t.Errorf("mar commits: %d", tl[2].Commits)
	}
	if tl[5].Commits != 1 {
		t.Errorf("jun commits: %d", tl[5].Commits)
	}
	if tl[1].Commits != 0 {
		t.Errorf("feb should be empty, got %d", tl[1].Commits)
	}
}

func TestBuildTimeline_Empty(t *testing.T) {
	if got := BuildTimeline(nil); got != nil {
		t.Errorf("empty input: got %v", got)
	}
}

func TestBusFactor_OneAuthorDominant(t *testing.T) {
	cons := []git.Contributor{
		{Name: "alice", Commits: 60},
		{Name: "bob", Commits: 30},
		{Name: "carol", Commits: 10},
	}
	bf, msg := BusFactor(cons)
	if bf != 1 {
		t.Errorf("expected bus factor 1, got %d", bf)
	}
	if msg == "" {
		t.Error("expected non-empty message")
	}
}

func TestBusFactor_TwoAuthorsNeeded(t *testing.T) {
	cons := []git.Contributor{
		{Name: "alice", Commits: 40},
		{Name: "bob", Commits: 30},
		{Name: "carol", Commits: 30},
	}
	bf, _ := BusFactor(cons)
	if bf != 2 {
		t.Errorf("expected bus factor 2, got %d", bf)
	}
}

func TestBusFactor_ThreeAuthorsNeeded(t *testing.T) {
	cons := []git.Contributor{
		{Name: "a", Commits: 30},
		{Name: "b", Commits: 25},
		{Name: "c", Commits: 25},
		{Name: "d", Commits: 20},
	}
	bf, _ := BusFactor(cons)
	// 30/100 = 30%; 30+25=55/100=55% → bf = 2
	if bf != 2 {
		t.Errorf("expected bus factor 2, got %d", bf)
	}
}

func TestBusFactor_Empty(t *testing.T) {
	bf, msg := BusFactor(nil)
	if bf != 0 || msg == "" {
		t.Errorf("empty: got (%d, %q)", bf, msg)
	}
}

func TestTimelineBarHeight_LogScaling(t *testing.T) {
	r := &Report{Timeline: []MonthBucket{
		{Label: "2026-01", Commits: 1},
		{Label: "2026-02", Commits: 100},
		{Label: "2026-03", Commits: 10},
	}}
	// With log scaling, the 100-commit bar should be tall, but the
	// 10-commit bar should still be visible (not flattened to 0).
	max := r.TimelineMax()
	if max != 100 {
		t.Fatalf("timeline max: %d", max)
	}
	low := r.TimelineBarHeight(1, 100)
	mid := r.TimelineBarHeight(10, 100)
	high := r.TimelineBarHeight(100, 100)
	if !(low < mid && mid < high) {
		t.Errorf("expected low<mid<high, got %d,%d,%d", low, mid, high)
	}
	if high != 100 {
		t.Errorf("max bar should fill height, got %d", high)
	}
	if low < 1 {
		t.Errorf("min bar should be at least 1px, got %d", low)
	}
}

func TestBuild_RejectsEmpty(t *testing.T) {
	_, err := Build("/tmp", "tmp", nil, nil, nil, nil, 0, "", "#fff")
	if err == nil {
		t.Error("expected error for empty commits")
	}
}

func TestBuild_PopulatesFields(t *testing.T) {
	d := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	cs := []git.Commit{
		{Hash: "aaa", Author: "alice", Email: "a@x", Time: d2, Subject: "second"},
		{Hash: "bbb", Author: "alice", Email: "a@x", Time: d, Subject: "first"},
	}
	cons := []git.Contributor{{Name: "alice", Email: "a@x", Commits: 2, FirstAt: d, LastAt: d2}}
	r, err := Build("/x", "x", cs, cons, nil, nil, 0, "", "#fff")
	if err != nil {
		t.Fatal(err)
	}
	if r.FirstCommit.Hash != "bbb" {
		t.Errorf("first commit: %s", r.FirstCommit.Hash)
	}
	if r.LastCommit.Hash != "aaa" {
		t.Errorf("last commit: %s", r.LastCommit.Hash)
	}
	if r.TotalCommits != 2 {
		t.Errorf("total commits: %d", r.TotalCommits)
	}
	if len(r.Timeline) != 6 {
		t.Errorf("timeline months: %d", len(r.Timeline))
	}
	if r.BusFactor != 1 {
		t.Errorf("bus factor: %d", r.BusFactor)
	}
}
