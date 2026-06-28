package git

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ShowCommit returns the full Commit record (including file touches) for a
// single SHA. Used for the "first commit" and "latest commit" cards.
func ShowCommit(repoPath, hash string) (Commit, error) {
	if hash == "" {
		return Commit{}, errors.New("ShowCommit: empty hash")
	}
	// Reuse the same machinery as Commits() but with --max-count=1 and a
	// hash filter.
	out, err := run(repoPath,
		"-C", repoPath, "log",
		"--format=%H%x00%an%x00%aI%x00%P%x00%s%x00%b%x00%cn%x00%ae%x00",
		"--name-status",
		"-n", "1", hash,
	)
	if err != nil {
		return Commit{}, fmt.Errorf("git show %s: %w", hash, err)
	}
	cs, err := parseCommitsWithFiles(out)
	if err != nil {
		return Commit{}, fmt.Errorf("parse commit %s: %w", hash, err)
	}
	if len(cs) == 0 {
		return Commit{}, fmt.Errorf("commit %s not found", hash)
	}
	return cs[0], nil
}

// FilesAtHEAD returns the list of files tracked in the named ref, sorted
// alphabetically. Used by the languages histogram in the single-ref
// report. For --base compare mode, use FilesAtRef(repoPath, baseRef)
// so the comparison is between the base ref's tree and HEAD's tree
// rather than between two HEAD reads.
//
// FilesAtHEAD reads the *committed* tree of HEAD via `git ls-tree`, not
// the user's working tree. It only matches the working tree when there
// are no uncommitted edits; LinesByFile is the function that actually
// touches the working tree (it reads bytes off disk).
func FilesAtHEAD(repoPath string) ([]string, error) {
	return FilesAtRef(repoPath, "HEAD")
}

// FilesAtRef returns the list of files tracked in the given ref, sorted
// alphabetically by name. Used by the languages histogram in both the
// single-ref report (ref=HEAD) and the --base compare mode (ref=baseRef).
// Reading from the ref's tree means the result is independent of any
// uncommitted working-tree edits the user has in progress.
func FilesAtRef(repoPath, ref string) ([]string, error) {
	if ref == "" {
		ref = "HEAD"
	}
	out, err := run(repoPath, "-C", repoPath, "ls-tree", "-r", "--name-only", ref)
	if err != nil {
		return nil, fmt.Errorf("git ls-tree %s: %w", ref, err)
	}
	lines := strings.Split(out, "\n")
	var files []string
	for _, l := range lines {
		l = strings.TrimRight(l, "\r")
		if l != "" {
			files = append(files, l)
		}
	}
	sort.Strings(files)
	return files, nil
}

// ReadmeText returns the first maxLines non-empty lines of the repo's
// README file, if one exists. Search order: README.md, README, README.txt.
// Returns "" if no README is found (not an error).
func ReadmeText(repoPath string, maxLines int) (string, error) {
	if maxLines <= 0 {
		return "", nil
	}
	for _, name := range []string{"README.md", "README", "README.txt"} {
		p := filepath.Join(repoPath, name)
		data, err := os.ReadFile(p)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return "", fmt.Errorf("read %s: %w", p, err)
		}
		return firstNonEmptyLines(string(data), maxLines), nil
	}
	return "", nil
}

// firstNonEmptyLines returns the first maxLines non-empty lines of s, joined
// with newlines, with trailing whitespace stripped.
func firstNonEmptyLines(s string, maxLines int) string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimRight(line, " \t\r")
		if strings.TrimSpace(trimmed) == "" {
			continue
		}
		out = append(out, trimmed)
		if len(out) >= maxLines {
			break
		}
	}
	return strings.Join(out, "\n")
}

// TotalLineCount sums line counts across the working-tree files in repoPath.
// It skips binary files by extension and continues past permission errors,
// returning whatever sum it managed to compute. An all-error case returns
// (0, nil) — the languages section is non-essential.
//
// This is best-effort. It does NOT use git plumbing, just a stat-and-count
// walk, because `git ls-files | xargs wc -l` would fork xargs and we want
// zero subprocess fan-out beyond git itself.
func TotalLineCount(repoPath string) (int64, error) {
	files, err := FilesAtHEAD(repoPath)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, rel := range files {
		if isBinaryExt(rel) {
			continue
		}
		abs := filepath.Join(repoPath, rel)
		data, err := os.ReadFile(abs)
		if err != nil {
			// Permission errors and missing submodules are non-fatal; we
			// surface them only by skipping.
			continue
		}
		total += int64(countLines(data))
	}
	return total, nil
}

// isBinaryExt returns true for extensions we treat as binary and skip when
// counting lines. This is intentionally a denylist rather than a detect-by-
// content check — fast and predictable.
func isBinaryExt(path string) bool {
	base := filepath.Base(path)
	dot := strings.LastIndexByte(base, '.')
	if dot < 0 {
		return false
	}
	switch strings.ToLower(base[dot+1:]) {
	case "png", "jpg", "jpeg", "gif", "webp", "ico", "bmp", "tiff",
		"pdf", "zip", "tar", "gz", "bz2", "xz", "7z", "rar",
		"mp3", "mp4", "wav", "ogg", "flac", "mov", "avi", "mkv",
		"woff", "woff2", "ttf", "otf", "eot",
		"exe", "dll", "so", "dylib", "o", "a", "class", "jar":
		return true
	}
	return false
}

// countLines returns the number of lines in data. A trailing newline does not
// add an empty line at the end (matches `wc -l`).
func countLines(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	n := strings.Count(string(data), "\n")
	if data[len(data)-1] != '\n' {
		n++
	}
	return n
}

// AggregateContributors rolls a commit list up into one record per author.
// Author identity is by email — the same human with two Git author names
// collapses into one contributor.
func AggregateContributors(commits []Commit) []Contributor {
	byEmail := map[string]*Contributor{}
	// Sort by time asc so FirstAt / LastAt are correct without min/max calls.
	for _, c := range commits {
		k := strings.ToLower(c.Email)
		if k == "" {
			k = strings.ToLower(c.Author)
		}
		cur, ok := byEmail[k]
		if !ok {
			cur = &Contributor{
				Name:    c.Author,
				Email:   c.Email,
				FirstAt: c.Time,
				LastAt:  c.Time,
			}
			byEmail[k] = cur
		}
		cur.Commits++
		if c.Time.Before(cur.FirstAt) {
			cur.FirstAt = c.Time
		}
		if c.Time.After(cur.LastAt) {
			cur.LastAt = c.Time
		}
	}
	out := make([]Contributor, 0, len(byEmail))
	for _, c := range byEmail {
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Commits > out[j].Commits })
	return out
}

// AggregateHotFiles returns the top N most-touched files across commits,
// ordered by touches desc. Primary author is the author with the most
// touches on that file. ties broken by LastModified desc.
func AggregateHotFiles(commits []Commit, topN int) []FileStat {
	type acc struct {
		FileStat
		authors map[string]int // email -> touches
	}
	m := map[string]*acc{}
	for _, c := range commits {
		for _, f := range c.Files {
			a, ok := m[f.Path]
			if !ok {
				a = &acc{
					FileStat: FileStat{Path: f.Path, LastModified: c.Time},
					authors:  map[string]int{},
				}
				m[f.Path] = a
			}
			a.Touches++
			a.authors[strings.ToLower(c.Email)]++
			if c.Time.After(a.LastModified) {
				a.LastModified = c.Time
			}
		}
	}
	out := make([]FileStat, 0, len(m))
	for _, a := range m {
		var bestEmail string
		best := -1
		for email, n := range a.authors {
			if n > best {
				best = n
				bestEmail = email
			}
		}
		// Resolve email -> author display name from any commit where it
		// appears; we look back through commits lazily here.
		name := bestEmail
		for _, c := range commits {
			if strings.EqualFold(c.Email, bestEmail) {
				name = c.Author
				break
			}
		}
		a.PrimaryAuthor = name
		out = append(out, a.FileStat)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Touches != out[j].Touches {
			return out[i].Touches > out[j].Touches
		}
		return out[i].LastModified.After(out[j].LastModified)
	})
	if topN > 0 && len(out) > topN {
		out = out[:topN]
	}
	return out
}

// AggregateLanguages groups files by extension and sums lines. Returns the top
// N languages by line count desc, plus an "other" bucket for the rest. Files
// without an extension are bucketed under "(no ext)".
func AggregateLanguages(files []string, linesByFile map[string]int64, topN int) []LangStat {
	byExt := map[string]*LangStat{}
	for _, f := range files {
		ext := filepath.Ext(f)
		if ext == "" {
			ext = "(no ext)"
		} else {
			ext = strings.ToLower(ext[1:]) // drop leading dot
		}
		cur, ok := byExt[ext]
		if !ok {
			cur = &LangStat{Extension: ext}
			byExt[ext] = cur
		}
		cur.Files++
		cur.Lines += linesByFile[f]
	}
	out := make([]LangStat, 0, len(byExt))
	for _, v := range byExt {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Lines > out[j].Lines })
	if topN > 0 && len(out) > topN {
		// Combine the tail into "(other)".
		var other LangStat
		other.Extension = "(other)"
		for _, v := range out[topN:] {
			other.Files += v.Files
			other.Lines += v.Lines
		}
		out = append(out[:topN], other)
	}
	return out
}

// LinesByFile reads file line counts on demand, reading from the working
// tree (the user's local disk). This is the "what's actually on my
// machine right now" view, which is what the single-ref report wants
// but is NOT what --base compare mode wants.
//
// Deprecated for compare mode: LinesByFile touches disk and reads
// whatever happens to be in the user's working tree, so a --base
// comparison would be between uncommitted working-tree bytes and
// themselves, not between the base ref's tree and HEAD's. Use
// LinesByFileAtRef(repoPath, baseRef) in the --base branch.
func LinesByFile(repoPath string) (map[string]int64, error) {
	return LinesByFileAtRef(repoPath, "HEAD")
}

// LinesByFileAtRef reads line counts for every file tracked in the given
// ref, by reading each blob via `git cat-file blob <oid>`. This makes
// the result independent of the user's working tree, so --base compare
// mode compares the base ref's tree to HEAD's tree rather than the
// working tree to itself. Binary files (per isBinaryExt) are skipped
// and reported as 0 lines, matching the working-tree semantics.
func LinesByFileAtRef(repoPath, ref string) (map[string]int64, error) {
	if ref == "" {
		ref = "HEAD"
	}
	files, err := FilesAtRef(repoPath, ref)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(files))
	for _, rel := range files {
		if isBinaryExt(rel) {
			out[rel] = 0
			continue
		}
		oid, err := run(repoPath, "-C", repoPath, "ls-tree", ref, "--", rel)
		if err != nil {
			out[rel] = 0
			continue
		}
		// `git ls-tree <ref> -- <path>` emits one line per matching
		// entry: "<mode> <type> <oid>\t<path>". For a tracked blob the
		// type is "blob" and the OID is what we want. Skip anything
		// that doesn't look like a regular blob (e.g. submodules, which
		// git reports as type "commit").
		fields := strings.Fields(oid)
		if len(fields) < 3 || fields[1] != "blob" {
			out[rel] = 0
			continue
		}
		oid = fields[2]
		data, err := run(repoPath, "-C", repoPath, "cat-file", "blob", oid)
		if err != nil {
			out[rel] = 0
			continue
		}
		out[rel] = int64(countLines([]byte(data)))
	}
	return out, nil
}

