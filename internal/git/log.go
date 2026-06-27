package git

import (
	"fmt"
	"strings"
	"time"
)

// Commits walks the commit log of repoPath in reverse-chronological order.
// The returned commits include their file touches (added/deleted counts).
//
// The implementation uses a single `git log` invocation with a custom
// pretty-print format and a NUL-byte separator between fields. We avoid
// `--follow` for renames: detecting renames across history is expensive and
// the marginal accuracy is not worth it for the "hot files" view.
func Commits(repoPath string, opts CommitOpts) ([]Commit, error) {
	args := []string{
		"-C", repoPath,
		"log",
		// Eight fields per commit, each terminated by NUL:
		//   hash, authorName, time, parents(space-sep), subject, body,
		//   committerName, email
		"--format=%H%x00%an%x00%aI%x00%P%x00%s%x00%b%x00%cn%x00%ae%x00",
		"--name-status",
	}

	if !opts.Since.IsZero() {
		args = append(args, fmt.Sprintf("--since=%s", opts.Since.UTC().Format("2006-01-02T15:04:05")))
	}
	if !opts.Until.IsZero() {
		args = append(args, fmt.Sprintf("--until=%s", opts.Until.UTC().Format("2006-01-02T15:04:05")))
	}
	if opts.AllRefs {
		args = append(args, "--all")
	} else {
		args = append(args, "HEAD")
	}
	if opts.MaxCount > 0 {
		args = append(args, fmt.Sprintf("--max-count=%d", opts.MaxCount))
	}
	if opts.Path != "" {
		args = append(args, "--", opts.Path)
	}

	out, err := run(repoPath, args...)
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}

	return parseCommitsWithFiles(out)
}

// parseCommitsWithFiles handles the joined output of
// `git log --format=...%x00 --name-status`. Per commit, the layout is:
//
//	<hash>\0<author>\0<time>\0<parents>\0<subject>\0<body>\0<committer>\0<email>\0\n\n<name-status block>
//
// We split the entire output on \0 and regroup into chunks of 8 fields. The
// final field (email) is followed by a \0 then "\n\n" then the name-status
// block. We attach that block to the same commit; the next chunk starts the
// next commit's record.
func parseCommitsWithFiles(out string) ([]Commit, error) {
	if out == "" {
		return nil, nil
	}
	// Trim the trailing newline git always emits.
	out = strings.TrimRight(out, "\n")

	parts := strings.Split(out, "\x00")
	if len(parts) < 8 {
		return nil, nil
	}

	var commits []Commit
	// parts has 8 fields per commit plus 1 trailing fragment (the name-status
	// block + "\n" + the next record's hash...). We iterate in groups of 8
	// and consume the next non-empty fragment as the file block for that
	// commit. The trailing fragment of the last commit may be empty.
	for i := 0; i+8 <= len(parts); i += 8 {
		hash := parts[i+0]
		author := parts[i+1]
		timeStr := parts[i+2]
		parentsStr := parts[i+3]
		subject := parts[i+4]
		body := parts[i+5]
		committer := parts[i+6] //nolint:unused // reserved for future use
		_ = committer
		email := parts[i+7]

		// Parse time.
		t, err := time.Parse(gitTimeLayout, timeStr)
		if err != nil {
			return nil, fmt.Errorf("parse time %q at index %d: %w", timeStr, i, err)
		}

		var parents []string
		if parentsStr != "" {
			parents = strings.Split(parentsStr, " ")
		}

		// The next fragment (parts[i+8]) is the name-status block for this
		// commit. It begins with a literal "\n\n" then "A\tpath\nM\tpath\n...".
		// If this is the last commit, parts[i+8] may not exist; treat as empty.
		var fileLines []string
		if i+8 < len(parts) {
			fileLines = extractNameStatusLines(parts[i+8])
		}
		// The file block for the next commit starts at parts[i+9]; we just
		// continue the loop and re-process. The file block always sits
		// *between* commit records because of the %x00 terminator on email.

		c := Commit{
			Hash:    hash,
			Author:  author,
			Time:    t,
			Parents: parents,
			Subject: subject,
			Body:    strings.TrimRight(body, "\n"),
			Email:   email,
		}
		if files, err := parseNameStatus(fileLines); err == nil {
			c.Files = files
		}
		commits = append(commits, c)
	}
	return commits, nil
}

// extractNameStatusLines strips the leading "\n\n" git inserts before the
// name-status block and returns the file lines. The trailing fragment may
// contain the next commit's name-status block as well (because %x00 only
// appears between fields, not before the file block), but since file lines
// always match the name-status shape, we stop at the first line that
// doesn't.
func extractNameStatusLines(frag string) []string {
	frag = strings.TrimLeft(frag, "\n")
	if frag == "" {
		return nil
	}
	lines := strings.Split(frag, "\n")
	var out []string
	for _, l := range lines {
		if l == "" {
			continue
		}
		if !isNameStatusLine(l) {
			// We've hit something that isn't a name-status record —
			// likely the start of the next commit (a 40-char hex SHA,
			// which has no tab). Stop.
			break
		}
		out = append(out, l)
	}
	return out
}

// isNameStatusLine reports whether line matches git's `--name-status` output.
// Patterns: "M\tpath", "A\tpath", "D\tpath", "T\tpath", and the renamed
// records "R<num>\told\tnew" and copied "C<num>\told\tnew".
func isNameStatusLine(line string) bool {
	if line == "" {
		return false
	}
	tab := strings.IndexByte(line, '\t')
	if tab <= 0 {
		return false
	}
	prefix := line[:tab]
	if len(prefix) == 0 || len(prefix) > 2 {
		return false
	}
	c := prefix[0]
	if c != 'A' && c != 'M' && c != 'D' && c != 'T' && c != 'C' && c != 'R' {
		return false
	}
	if len(prefix) == 2 && (prefix[1] < '0' || prefix[1] > '9') {
		return false
	}
	return true
}

// parseNameStatus converts git name-status lines into FileTouch records.
// Rename and copy records collapse to a touch on the *new* path; we don't
// track per-file added/deleted here because `--name-status` doesn't emit
// them (and computing diffstat for every touched file is far too expensive
// for the "hot files" view).
func parseNameStatus(lines []string) ([]FileTouch, error) {
	var out []FileTouch
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		ft := FileTouch{Path: parts[len(parts)-1]}
		if len(parts) == 3 && len(parts[0]) > 0 && (parts[0][0] == 'R' || parts[0][0] == 'C') {
			// R<num>\told\tnew or C<num>\told\tnew — path is the new name.
			ft.Path = parts[2]
			out = append(out, ft)
			continue
		}
		out = append(out, ft)
	}
	return out, nil
}
