# Repository Guidelines

## Project Structure & Module Organization
- `svc/auth/`: Auth service (`api.go`, `handler.go`, `service.go`, `repo.go`, `dto.go`, `errors.go`).
- `pkg/`: Reusable libraries (`authn`, `middleware`, `rate_limit`, `session`).
- `coredb/`: Database setup and SQL migrations (`migrations/*.sql`, `database.go`).
- `scripts/`: Ops/dev scripts (e.g., `seed_dev.sql`).
- Root: `encore.app`, `go.mod`, `README.md`.

## Build, Test, and Development Commands
- Run locally (Encore): `encore run` — starts the API and a local dev Postgres.
- Unit tests: `go test ./...` — runs all tests.
- Coverage: `go test ./... -coverprofile=cover.out && go tool cover -func=cover.out`.
- Vet (static checks): `go vet ./...`.
- Format: `gofmt -s -w .` (optionally `goimports -w .`).

## Coding Style & Naming Conventions
- Go formatting: keep code `gofmt`-clean; no custom styles.
- Packages: short, all lowercase, no underscores (e.g., `authn`, `middleware`).
- Files: snake_case (e.g., `email_verification.go`); tests as `*_test.go`.
- Exports: `CamelCase` for types/functions; interfaces named by behavior (e.g., `TokenStore`).
- Errors: wrap with context; prefer sentinel vars in `errors.go` per package.

## Testing Guidelines
- Framework: standard `testing` package.
- Location: tests colocated with code under `pkg/` and services under `svc/`.
- Names: `TestXxx` for units; table-driven tests preferred.
- Coverage target: aim ≥80% in `pkg/*` packages critical to auth/rate limiting.
- Run subsets: `go test ./pkg/authn -run TestJWT`.

## Commit & Pull Request Guidelines
- Commits: concise, imperative subject (≤72 chars), optional body explaining why.
- Scope: keep changes focused; separate refactors from behavior changes.
- PRs: include description, linked issues (e.g., `Closes #123`), test evidence, and examples (e.g., `curl` for new endpoints).
- CI hygiene: run `go vet`, `gofmt`, and `go test ./...` before opening.

## Security & Configuration Tips
- Secrets: never commit; use Encore secrets/env vars. Rotate if leaked.
- DB: apply SQL in `coredb/migrations` via dev run; use `scripts/seed_dev.sql` for local data if needed.
- Auth: avoid logging credentials/tokens; prefer structured logs without PII.
