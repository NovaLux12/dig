package git

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixtureRepo builds a tiny throwaway git repo with three commits across two
// authors, returning its path. dig is read-only on target repos, but tests
// own the directory they create.
func fixtureRepo(t *testing.T) string {
	t.Helper()
	if err := Available(); err != nil {
		t.Skipf("git not installed: %v", err)
	}
	dir := t.TempDir()
	mustRun := func(args ...string) {
		full := append([]string{"-C", dir}, args...)
		c := exec.Command("git", full...)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(full, " "), err, out)
		}
	}
	mustRun("init", "-q", "-b", "main")
	mustRun("config", "user.email", "alice@example.com")
	mustRun("config", "user.name", "Alice")
	mustRun("config", "commit.gpgsign", "false")

	writeFile := func(name, body string) {
		full := filepath.Join(dir, name)
		if err := exec.Command("bash", "-c", "mkdir -p '"+filepath.Dir(full)+"'").Run(); err != nil {
			t.Fatal(err)
		}
		if err := exec.Command("bash", "-c", "printf %s "+shellQuote(body)+" > '"+full+"'").Run(); err != nil {
			t.Fatal(err)
		}
	}
	commitAs := func(msg, when, who, email string) {
		mustRun("add", ".")
		c := exec.Command("git", "-C", dir, "commit", "-q", "-m", msg,
			"--author="+who+" <"+email+">",
			"--date="+when)
		// `git commit --date` only sets the committer timestamp; `git log
		// --since` filters on the author date. Set both via env so the
		// fixture's dates are honoured end-to-end.
		c.Env = append(c.Environ(), "GIT_AUTHOR_DATE="+when, "GIT_COMMITTER_DATE="+when)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("commit %q: %v: %s", msg, err, out)
		}
	}

	// Three commits, each preceded by a content change so the commit isn't
	// a no-op.
	writeFile("main.go", "package x\nfunc main() {}\n")
	writeFile("util.go", "package x\nfunc F() {}\n")
	writeFile("README.md", "# x\n\nhello\n")
	commitAs("init", "2026-06-01T12:00:00+00:00", "Alice", "alice@example.com")

	writeFile("main.go", "package x\nfunc main() { println(1) }\n")
	commitAs("add line", "2026-06-02T12:00:00+00:00", "Bob", "bob@example.com")

	writeFile("util.go", "package x\nfunc F() { println(1) }\nfunc G() {}\n")
	commitAs("extend util", "2026-06-03T12:00:00+00:00", "Bob", "bob@example.com")

	return dir
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func TestIsRepo(t *testing.T) {
	dir := fixtureRepo(t)
	if err := IsRepo(dir); err != nil {
		t.Fatalf("expected repo, got %v", err)
	}
}

func TestIsRepo_NotARepo(t *testing.T) {
	dir := t.TempDir()
	if err := IsRepo(dir); err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestCommits_RoundTrip(t *testing.T) {
	dir := fixtureRepo(t)
	cs, err := Commits(dir, CommitOpts{})
	if err != nil {
		t.Fatalf("Commits: %v", err)
	}
	if len(cs) != 3 {
		t.Fatalf("want 3 commits, got %d", len(cs))
	}
	// Reverse-chronological: newest first.
	if cs[0].Time.Before(cs[len(cs)-1].Time) {
		t.Errorf("expected reverse-chrono order")
	}
	if cs[0].Author != "Bob" {
		t.Errorf("latest commit author: want Bob, got %q", cs[0].Author)
	}
	if cs[0].Subject != "extend util" {
		t.Errorf("latest commit subject: got %q", cs[0].Subject)
	}
	if len(cs[0].Files) == 0 {
		t.Errorf("expected file touches on latest commit")
	}
}

func TestCommits_Since(t *testing.T) {
	dir := fixtureRepo(t)
	cutoff, _ := time.Parse(time.RFC3339, "2026-06-02T00:00:00Z")
	cs, err := Commits(dir, CommitOpts{Since: cutoff})
	if err != nil {
		t.Fatalf("Commits: %v", err)
	}
	if len(cs) != 2 {
		t.Errorf("since cutoff: want 2 commits, got %d", len(cs))
	}
}

func TestShowCommit(t *testing.T) {
	dir := fixtureRepo(t)
	cs, err := Commits(dir, CommitOpts{MaxCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	got, err := ShowCommit(dir, cs[0].Hash)
	if err != nil {
		t.Fatal(err)
	}
	if got.Hash != cs[0].Hash {
		t.Errorf("hash mismatch: %s vs %s", got.Hash, cs[0].Hash)
	}
	if got.Subject != cs[0].Subject {
		t.Errorf("subject mismatch: %q vs %q", got.Subject, cs[0].Subject)
	}
}

func TestFilesAtHEAD(t *testing.T) {
	dir := fixtureRepo(t)
	files, err := FilesAtHEAD(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"main.go": true, "util.go": true, "README.md": true}
	for _, f := range files {
		delete(want, f)
	}
	if len(want) > 0 {
		t.Errorf("missing files: %v (got %v)", want, files)
	}
}

func TestReadmeText(t *testing.T) {
	dir := fixtureRepo(t)
	got, err := ReadmeText(dir, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "# x") {
		t.Errorf("expected README heading, got %q", got)
	}
}

func TestAggregateContributors(t *testing.T) {
	dir := fixtureRepo(t)
	cs, err := Commits(dir, CommitOpts{})
	if err != nil {
		t.Fatal(err)
	}
	cons := AggregateContributors(cs)
	if len(cons) != 2 {
		t.Fatalf("want 2 contributors, got %d", len(cons))
	}
	if cons[0].Name == cons[1].Name {
		t.Errorf("expected distinct contributors, both %q", cons[0].Name)
	}
}

func TestAggregateHotFiles(t *testing.T) {
	dir := fixtureRepo(t)
	cs, err := Commits(dir, CommitOpts{})
	if err != nil {
		t.Fatal(err)
	}
	hot := AggregateHotFiles(cs, 25)
	if len(hot) == 0 {
		t.Fatal("want at least one hot file")
	}
	for _, h := range hot {
		if h.Touches == 0 {
			t.Errorf("hot file with zero touches: %+v", h)
		}
	}
}

func TestAggregateLanguages(t *testing.T) {
	dir := fixtureRepo(t)
	files, _ := FilesAtHEAD(dir)
	lines, err := LinesByFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	langs := AggregateLanguages(files, lines, 5)
	has := map[string]bool{}
	for _, l := range langs {
		has[l.Extension] = true
	}
	if !has["go"] {
		t.Errorf("expected go in languages, got %+v", langs)
	}
}
