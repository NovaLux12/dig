package report

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/NovaLux12/dig/internal/analyze"
	"github.com/NovaLux12/dig/internal/git"
)

func sampleReport() *analyze.Report {
	now := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	d1 := time.Date(2026, 6, 27, 11, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	d3 := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	first := git.Commit{
		Hash:    "1111111111111111111111111111111111111111",
		Author:  "alice",
		Email:   "alice@example.com",
		Time:    d3,
		Subject: "init",
		Files:   []git.FileTouch{{Path: "main.go"}, {Path: "README.md"}},
	}
	latest := git.Commit{
		Hash:    "2222222222222222222222222222222222222222",
		Author:  "bob",
		Email:   "bob@example.com",
		Time:    d1,
		Subject: "wire up dig",
		Files:   []git.FileTouch{{Path: "main.go"}, {Path: "internal/git/log.go"}},
	}
	commits := []git.Commit{latest, {
		Hash:    "3333333333333333333333333333333333333333",
		Author:  "alice",
		Email:   "alice@example.com",
		Time:    d2,
		Subject: "second",
		Files:   []git.FileTouch{{Path: "main.go"}},
	}, first}

	return &analyze.Report{
		RepoName:     "dig",
		RepoPath:     "/tmp/dig",
		FirstCommit:  first,
		LastCommit:   latest,
		TotalCommits: len(commits),
		Contributors: []git.Contributor{
			{Name: "alice", Email: "alice@example.com", Commits: 2, FirstAt: d3, LastAt: d2},
			{Name: "bob", Email: "bob@example.com", Commits: 1, FirstAt: d1, LastAt: d1},
		},
		HotFiles: []git.FileStat{
			{Path: "main.go", Touches: 3, LastModified: d1, PrimaryAuthor: "alice"},
			{Path: "internal/git/log.go", Touches: 1, LastModified: d1, PrimaryAuthor: "bob"},
			{Path: "README.md", Touches: 1, LastModified: d3, PrimaryAuthor: "alice"},
		},
		Languages: []git.LangStat{
			{Extension: "go", Files: 2, Lines: 100},
			{Extension: "md", Files: 1, Lines: 10},
		},
		FileCount:   3,
		Readme:      "# dig\n\nA code-archaeology report generator.\n",
		GeneratedAt: now,
		Accent:      "#7aa2f7",
		// Timeline, BusFactor populated by Build.
	}
}

// TestRender_Smoke just makes sure Render produces non-empty HTML without
// panicking. The golden-file test below verifies exact bytes.
func TestRender_Smoke(t *testing.T) {
	r := sampleReport()
	r.Timeline = analyze.BuildTimeline([]git.Commit{
		{Hash: "x", Time: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)},
		{Hash: "y", Time: time.Date(2026, 6, 27, 0, 0, 0, 0, time.UTC)},
	})
	r.BusFactor, r.BusFactorMsg = analyze.BusFactor(r.Contributors)

	out, err := Render(r, nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.Contains(out, []byte("<!doctype html>")) {
		t.Errorf("output missing doctype")
	}
	if !bytes.Contains(out, []byte("dig")) {
		t.Errorf("output missing repo name")
	}
}

// TestRender_WithDelta verifies the delta section renders when a Delta is
// passed. We don't golden-compare the delta output (commits/time data
// shift), only assert the structural elements are present.
func TestRender_WithDelta(t *testing.T) {
	r := sampleReport()
	r.Timeline = analyze.BuildTimeline([]git.Commit{
		{Hash: "x", Time: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)},
		{Hash: "y", Time: time.Date(2026, 6, 27, 0, 0, 0, 0, time.UTC)},
	})
	r.BusFactor, r.BusFactorMsg = analyze.BusFactor(r.Contributors)

	// Build a base with fewer commits so there's a non-trivial delta.
	d1 := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	base := &analyze.Report{
		RepoName:     "dig",
		TotalCommits: 1,
		Commits: []git.Commit{
			{Hash: "1111111111111111111111111111111111111111", Author: "alice",
				Email: "alice@example.com", Time: d1, Subject: "init"},
		},
		Contributors: []git.Contributor{
			{Name: "alice", Email: "alice@example.com", Commits: 1, FirstAt: d1, LastAt: d1},
		},
		BusFactor: 1,
	}
	delta := analyze.Compare(base, r, "v0.1.0", "HEAD")

	out, err := Render(r, delta)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{
		"Changes since v0.1.0",
		"Commits added",
		"Commits removed",
		"New contributors",
		"Departed contributors",
		"Bus factor",
		"bob", // new contributor
	} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("delta output missing %q", want)
		}
	}
}

// TestRender_Golden renders the same fixture and compares to a checked-in
// golden file. The GeneratedAt field is normalised before comparison so the
// test is deterministic across runs.
//
// To regenerate the golden file, run:
//
//	go test ./internal/report/ -update-golden
func TestRender_Golden(t *testing.T) {
	r := sampleReport()
	r.Timeline = analyze.BuildTimeline([]git.Commit{
		{Hash: "x", Time: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)},
		{Hash: "y", Time: time.Date(2026, 6, 27, 0, 0, 0, 0, time.UTC)},
	})
	r.BusFactor, r.BusFactorMsg = analyze.BusFactor(r.Contributors)

	got, err := Render(r, nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// Normalise GeneratedAt — the timestamp is the only non-deterministic
	// field. We rewrite it to a fixed value before comparison.
	got = bytes.ReplaceAll(got, []byte(`Generated by <a href="https://github.com/NovaLux12/dig">dig</a> on 2026-06-27T12:00:00Z`), []byte(`<NORMALISED-GENERATED-AT>`))

	goldenPath := filepath.Join("testdata", "golden.html")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Skip("golden file updated")
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run with UPDATE_GOLDEN=1 to create): %v", err)
	}
	// Compare line-by-line so a diff points at the right line.
	gotLines := strings.Split(string(got), "\n")
	wantLines := strings.Split(string(want), "\n")
	if len(gotLines) != len(wantLines) {
		t.Fatalf("line count: got %d, want %d", len(gotLines), len(wantLines))
	}
	for i := range gotLines {
		if gotLines[i] != wantLines[i] {
			t.Errorf("line %d differs:\n got: %q\nwant: %q", i+1, gotLines[i], wantLines[i])
			if i > 5 {
				t.Errorf("(further diffs elided)")
				break
			}
		}
	}
}
