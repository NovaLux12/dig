// Command dig generates a self-contained HTML code-archaeology report from
// a local git repository.
//
// Usage:
//
//	dig <repo-path>
//	dig <repo-path> --out report.html
//	dig <repo-path> --accent #ff5577
//	dig <repo-path> --since 12mo
//
// dig is read-only on the target repo. It never modifies, stages, or commits
// anything. The output is one HTML file with all CSS and SVG embedded; it
// works offline and renders in any modern browser.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/NovaLux12/dig/internal/analyze"
	"github.com/NovaLux12/dig/internal/git"
	"github.com/NovaLux12/dig/internal/report"
)

// version is overridden via -ldflags at release time.
var version = "dev"

const (
	exitOK              = 0
	exitNotARepo        = 1
	exitGitNotInstalled = 2
	exitIO              = 3
	exitUsage           = 64
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("dig", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		outPath  string
		accent   string
		sinceStr string
		allRefs  bool
		baseRef  string
		showVer  bool
		showHelp bool
	)
	fs.StringVar(&outPath, "out", "dig-report.html", "output HTML file path")
	fs.StringVar(&accent, "accent", "#7aa2f7", "accent colour (hex)")
	fs.StringVar(&sinceStr, "since", "", "restrict analysis to commits after this time (e.g. 12mo, 2024-01-01)")
	fs.BoolVar(&allRefs, "all", false, "walk all refs (branches and tags) instead of just HEAD")
	fs.StringVar(&baseRef, "base", "", "compare against this ref (branch, tag, or SHA prefix). Emits a delta report. Empty = no compare.")
	fs.BoolVar(&showVer, "version", false, "print version and exit")
	fs.BoolVar(&showHelp, "help", false, "show usage")

	fs.Usage = func() {
		fmt.Fprintf(stderr, "dig — generate a self-contained HTML code-archaeology report from a git repo.\n\n")
		fmt.Fprintf(stderr, "Usage:\n  dig <repo-path> [flags]\n\nFlags:\n")
		fs.PrintDefaults()
		fmt.Fprintf(stderr, "\nExit codes:\n  %d success\n  %d not a git repo\n  %d git not installed\n  %d I/O error\n", exitOK, exitNotARepo, exitGitNotInstalled, exitIO)
		fmt.Fprintf(stderr, "\nExamples:\n  dig ../my-repo\n  dig --base v1.0 --out since-v1.html ../my-repo\n  dig --since 12mo ../my-repo\n")
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if showHelp {
		fs.Usage()
		return exitOK
	}
	if showVer {
		fmt.Fprintf(stdout, "dig %s\n", version)
		return exitOK
	}

	if fs.NArg() < 1 {
		fs.Usage()
		return exitUsage
	}
	repoPath := fs.Arg(0)
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		fmt.Fprintf(stderr, "dig: resolve path %q: %v\n", repoPath, err)
		return exitIO
	}

	if err := git.Available(); err != nil {
		fmt.Fprintf(stderr, "dig: %v\n", err)
		return exitGitNotInstalled
	}
	if err := git.IsRepo(abs); err != nil {
		fmt.Fprintf(stderr, "dig: %q is not a git repository\n", repoPath)
		return exitNotARepo
	}

	since, err := parseSince(sinceStr)
	if err != nil {
		fmt.Fprintf(stderr, "dig: invalid --since: %v\n", err)
		return exitUsage
	}

	// Fetch the data.
	commits, err := git.Commits(abs, git.CommitOpts{
		Since:   since,
		AllRefs: allRefs,
	})
	if err != nil {
		fmt.Fprintf(stderr, "dig: walk commits: %v\n", err)
		return exitIO
	}
	if len(commits) == 0 {
		// Two distinct failure modes collapse to len==0 here:
		//   1. The repository is truly empty (no commits at all).
		//   2. The repository has commits, but --since filtered them all out.
		// Branch on whether --since was actually set so each gets its
		// own exit code and message. exitNotARepo is right for (1);
		// exitUsage is right for (2) — a "your filter excluded
		// everything" condition is a user error, not a path error.
		if since.IsZero() {
			fmt.Fprintf(stderr, "dig: repository has no commits\n")
			return exitNotARepo
		}
		fmt.Fprintf(stderr, "dig: no commits match --since=%s (try an earlier date or drop --since)\n",
			since.UTC().Format("2006-01-02T15:04:05Z"))
		return exitUsage
	}

	contributors := git.AggregateContributors(commits)
	hotFiles := git.AggregateHotFiles(commits, 25)
	filesAtHEAD, err := git.FilesAtHEAD(abs)
	if err != nil {
		fmt.Fprintf(stderr, "dig: list files: %v\n", err)
		return exitIO
	}
	linesByFile, err := git.LinesByFile(abs)
	if err != nil {
		// Non-fatal — languages histogram will just be sparse.
		fmt.Fprintf(stderr, "dig: line counts: %v (continuing with partial data)\n", err)
		linesByFile = map[string]int64{}
	}
	languages := git.AggregateLanguages(filesAtHEAD, linesByFile, 15)

	readme, err := git.ReadmeText(abs, 80)
	if err != nil {
		fmt.Fprintf(stderr, "dig: read README: %v (continuing without)\n", err)
		readme = ""
	}

	repoName := strings.TrimSuffix(filepath.Base(abs), ".git")

	r, err := analyze.Build(abs, repoName, commits, contributors, hotFiles, languages,
		len(filesAtHEAD), readme, accent)
	if err != nil {
		fmt.Fprintf(stderr, "dig: analyse: %v\n", err)
		return exitIO
	}

	var delta *analyze.Delta
	if baseRef != "" {
		baseCommits, err := git.Commits(abs, git.CommitOpts{
			Since:   since,
			AllRefs: false,
			Ref:     baseRef,
		})
		if err != nil {
			fmt.Fprintf(stderr, "dig: walk base ref %q: %v\n", baseRef, err)
			return exitIO
		}
		if len(baseCommits) == 0 {
			fmt.Fprintf(stderr, "dig: base ref %q has no commits (does it exist?)\n", baseRef)
			return exitNotARepo
		}
		baseContributors := git.AggregateContributors(baseCommits)
		baseHotFiles := git.AggregateHotFiles(baseCommits, 25)
		// Use the ref-parameterised variants so the comparison reads
		// the base ref's tree (commits AND file content), not the
		// user's working tree. Without this the language section and
		// the file-count metric would be derived from uncommitted
		// edits versus themselves, not from baseRef versus HEAD.
		baseFilesAtHEAD, err := git.FilesAtRef(abs, baseRef)
		if err != nil {
			fmt.Fprintf(stderr, "dig: list files at base: %v\n", err)
			return exitIO
		}
		baseLinesByFile, err := git.LinesByFileAtRef(abs, baseRef)
		if err != nil {
			fmt.Fprintf(stderr, "dig: line counts at base: %v (continuing with partial data)\n", err)
			baseLinesByFile = map[string]int64{}
		}
		baseLanguages := git.AggregateLanguages(baseFilesAtHEAD, baseLinesByFile, 15)
		baseReport, err := analyze.Build(abs, repoName, baseCommits, baseContributors,
			baseHotFiles, baseLanguages, len(baseFilesAtHEAD), "", accent)
		if err != nil {
			fmt.Fprintf(stderr, "dig: analyse base: %v\n", err)
			return exitIO
		}
		delta = analyze.Compare(baseReport, r, baseRef, "HEAD")
	}

	html, err := report.Render(r, delta)
	if err != nil {
		fmt.Fprintf(stderr, "dig: render: %v\n", err)
		return exitIO
	}

	if err := os.WriteFile(outPath, html, 0o644); err != nil {
		fmt.Fprintf(stderr, "dig: write %s: %v\n", outPath, err)
		return exitIO
	}
	fmt.Fprintf(stdout, "wrote %s (%s bytes)\n", outPath, comma(int64(len(html))))
	return exitOK
}

// parseSince interprets values like "12mo", "2y", "30d", or RFC3339 dates.
// Empty input returns the zero time (no filter).
func parseSince(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	// Relative shorthand: digits + unit suffix (d/w/mo/y).
	if len(s) >= 2 {
		n := 0
		i := 0
		for ; i < len(s); i++ {
			c := s[i]
			if c < '0' || c > '9' {
				break
			}
			n = n*10 + int(c-'0')
		}
		unit := strings.ToLower(s[i:])
		if n > 0 {
			now := time.Now().UTC()
			switch unit {
			case "d":
				return now.AddDate(0, 0, -n), nil
			case "w":
				return now.AddDate(0, 0, -7*n), nil
			case "mo":
				return now.AddDate(0, -n, 0), nil
			case "y":
				return now.AddDate(-n, 0, 0), nil
			}
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised --since value %q (try 12mo, 2y, 30d, or RFC3339)", s)
}

func comma(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
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
	return b.String()
}
