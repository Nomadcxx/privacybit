# Privacybit CI Design

Issue: [#24](https://github.com/Nomadcxx/privacybit/issues/24)

## Decision

Add one GitHub Actions workflow at `.github/workflows/ci.yml`. Run it for pull
requests and pushes to `main`. Use the Go version in `go.mod` and grant the job
read-only access to repository contents.

Run formatting, tests, vet, and build in one Ubuntu job:

- `gofmt` must report no changed Go files.
- `go test -count=1 -p 2 ./...` runs the full suite without cached results.
- `go vet -p 2 ./...` checks every package.
- `go build -p 2 ./...` compiles every package.

Keep the checks sequential so one runner uses bounded resources. The workflow
does not publish releases or request write permissions.

## Acceptance

Every pull request and push to `main` runs all four checks with the Go version
declared in `go.mod`. Any failed command fails the workflow.
