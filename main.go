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
		showVer  bool
		showHelp bool
	)
	fs.StringVar(&outPath, "out", "dig-report.html", "output HTML file path")
	fs.StringVar(&accent, "accent", "#7aa2f7", "accent colour (hex)")
	fs.StringVar(&sinceStr, "since", "", "restrict analysis to commits after this time (e.g. 12mo, 2024-01-01)")
	fs.BoolVar(&allRefs, "all", false, "walk all refs (branches and tags) instead of just HEAD")
	fs.BoolVar(&showVer, "version", false, "print version and exit")
	fs.BoolVar(&showHelp, "help", false, "show usage")

	fs.Usage = func() {
		fmt.Fprintf(stderr, "dig — generate a self-contained HTML code-archaeology report from a git repo.\n\n")
		fmt.Fprintf(stderr, "Usage:\n  dig <repo-path> [flags]\n\nFlags:\n")
		fs.PrintDefaults()
		fmt.Fprintf(stderr, "\nExit codes:\n  %d success\n  %d not a git repo\n  %d git not installed\n  %d I/O error\n", exitOK, exitNotARepo, exitGitNotInstalled, exitIO)
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
		fmt.Fprintf(stderr, "dig: repository has no commits\n")
		return exitNotARepo
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

	html, err := report.Render(r)
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
