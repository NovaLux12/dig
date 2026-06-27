// Package git is a thin wrapper around the git CLI.
//
// dig shells out to the system `git` binary rather than reimplementing commit
// parsing, rename detection, and diffstat calculation in Go. The git binary is
// the source of truth for these things; reimplementing them introduces drift
// and bugs. The trade-off is a runtime dependency on a `git` binary on $PATH.
//
// All functions in this package are read-only — they never modify the target
// repository.
package git

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ErrNotARepository is returned when the given path is not inside a git
// working tree.
var ErrNotARepository = errors.New("not a git repository")

// ErrGitNotInstalled is returned when the `git` binary is not on $PATH.
var ErrGitNotInstalled = errors.New("git binary not found on $PATH")

// Commit is a single parsed git log entry.
type Commit struct {
	Hash    string    // full 40-character SHA-1
	Author  string    // display name
	Email   string    // author email
	Time    time.Time // author timestamp (committer date is ignored)
	Subject string    // first line of the commit message
	Body    string    // remainder of the message after the subject line
	Parents []string  // parent commit SHAs (empty for the root commit)
	Files   []FileTouch
}

// FileTouch records one path's diffstat in a single commit.
type FileTouch struct {
	Path    string
	Added   int
	Deleted int
}

// CommitOpts filters the set of commits returned by Commits.
type CommitOpts struct {
	// Since, if non-zero, restricts commits to those after this time.
	Since time.Time
	// Until, if non-zero, restricts commits to those before this time.
	Until time.Time
	// AllRefs, if true, walks all refs (branches and tags), not just HEAD.
	AllRefs bool
	// MaxCount, if positive, caps the number of commits returned.
	MaxCount int
	// Path, if non-empty, restricts commits to those touching this path.
	Path string
	// Ref, if non-empty, walks this specific ref (branch, tag, or SHA
	// prefix). Defaults to HEAD when AllRefs is false and Ref is empty.
	// Ignored when AllRefs is true.
	Ref string
}

// Contributor aggregates per-author commit counts and times.
type Contributor struct {
	Name    string
	Email   string
	Commits int
	FirstAt time.Time
	LastAt  time.Time
}

// FileStat aggregates per-file touch counts and authorship.
type FileStat struct {
	Path          string
	Touches       int
	LastModified  time.Time
	PrimaryAuthor string
}

// LangStat groups files by extension for the languages histogram.
type LangStat struct {
	Extension string
	Files     int
	Lines     int64
}

// Available reports whether the `git` binary is on $PATH and runnable.
func Available() error {
	_, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrGitNotInstalled, err)
	}
	return nil
}

// IsRepo returns nil if path is inside a git working tree, otherwise
// ErrNotARepository.
func IsRepo(path string) error {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--is-inside-work-tree")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", ErrNotARepository, strings.TrimSpace(string(out)))
	}
	if strings.TrimSpace(string(out)) != "true" {
		return ErrNotARepository
	}
	return nil
}

// run is the shared subprocess runner. It returns combined stdout; combined
// stderr is folded into the error message for debuggability.
func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %v: %s",
			strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// gitTimeLayout is the format git uses for --format=%cI / %aI. We use
// time.RFC3339 because git emits `Z` for UTC and a numeric offset
// (e.g. `+01:00`) for non-UTC timestamps; RFC3339 accepts both.
const gitTimeLayout = time.RFC3339

// parseCommitLog parses the output of `git log` with a custom --format into
// a slice of Commit. The separator is a single NUL byte; newlines inside the
// message body are preserved.
func parseCommitLog(out string) ([]Commit, error) {
	if out == "" {
		return nil, nil
	}
	// We separate commits with a sentinel rather than relying on git's
	// default blank-line separator, because commit bodies can contain
	// blank lines that would otherwise confuse a line-based parser.
	records := strings.Split(out, "\x00")
	var commits []Commit
	for _, rec := range records {
		rec = strings.TrimRight(rec, "\n")
		if rec == "" {
			continue
		}
		// Field separator inside a record is "\n".
		lines := strings.SplitN(rec, "\n", 9)
		if len(lines) < 8 {
			return nil, fmt.Errorf("malformed git log record: %q", rec)
		}
		t, err := time.Parse(gitTimeLayout, lines[2])
		if err != nil {
			return nil, fmt.Errorf("parse commit time %q: %w", lines[2], err)
		}
		var parents []string
		if lines[3] != "" {
			parents = strings.Split(lines[3], " ")
		}
		subject := lines[4]
		body := strings.TrimPrefix(lines[5], "\n")
		c := Commit{
			Hash:    lines[0],
			Author:  lines[1],
			Time:    t,
			Parents: parents,
			Subject: subject,
			Body:    body,
		}
		// The author email is the email field at index 7; index 6 is the
		// committer name (which we ignore).
		c.Email = lines[7]
		// Files is empty for log records; callers that need file touches
		// use ShowCommit or walk the log themselves with --name-only.
		commits = append(commits, c)
	}
	return commits, nil
}
