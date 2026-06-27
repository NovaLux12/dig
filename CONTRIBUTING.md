# Contributing

## Workflow

1. Fork the repo, branch from `main`.
2. Make your change. Keep it focused — one PR, one thing.
3. Verify locally:
   ```
   go build -o dig .
   ./dig . --out dig-self-report.html
   go test ./...
   gofmt -l .   # must be empty
   ```
4. Open a PR. CI runs the same checks plus a Linux build.

## What `dig` should not do

- Pull network resources in any codepath (build time or run time).
- Take a runtime dependency on a non-stdlib Go module.
- Modify, stage, or commit anything in the target repo.
- Render anything that requires a JavaScript framework to view.

If your change would break any of the above, the change belongs in a fork,
not here.

## Test data

`internal/git/testdata/` is a real git repo created in `TestMain`. It's
small and self-contained. Add fixtures there only when the existing ones
can't express the case you need.

## Report golden file

`internal/report/testdata/golden.html` is the reference output for the
golden-file test. If you intentionally change the rendered output, regenerate
the golden file and commit it in the same PR. Don't normalise differences
in the test instead.

## Commit messages

Conventional Commits (`feat:`, `fix:`, `docs:`, `test:`, `refactor:`,
`chore:`). Imperative mood, lowercase subject, no period. Body explains
the *why*, not the *what*.