# Agent Guidelines

## Stack
- Backend: Go, Gin
- Frontend: HTML templates + Bootstrap + vanilla JS
- Storage: Redis for all storage needs tied to user id.
- Auth: OAuth2 (Google + GitHub), cookie sessions

## Project Rules
- Keep handlers thin: validation + call service layer only.
- Prefer standard library where possible. Avoid heavy ORMs.

## Project Structure
- Root docs live in `README.md` and `AGENTS.md`.
- Source code is expected to be Go; place packages under `cmd/` (entrypoints) and `internal/` or `pkg/` (shared logic).
- Tests should live next to the code they cover using `_test.go` filenames.
- Environment configuration is stored in `.env`; do not commit secrets. Document requirements in `README.md`.

## Coding Style
- Use tabs for all code formatting and indentation.
- Use `gofmt` formatting and standard Go style.
- Prefer small, testable functions and explicit error handling.
- Use `camelCase` for variables, `PascalCase` for exported identifiers, and `SNAKE_CASE` only for env vars.
- Keep edits minimal and avoid adding new dependencies without discussion.

## Commands
- go mod tidy
- gofmt -w .
- go test ./...
- go test -race ./...
- go vet ./...
- (optional) golangci-lint run

## Frontend
- Use server-rendered templates in /web/templates.
- Use Bootstrap components, minimal custom CSS.
- Use progressive enhancement; pages should work without JS when feasible.

## Security
- Session cookies: Secure, HttpOnly, SameSite=Lax (or Strict if possible).
- Invite tokens: store only SHA-256 hash; tokens expire.
- Validate all IDs and ownership.

## Testing
- Use Go’s `testing` package for unit tests.
- Name tests `TestXxx` and table-driven tests with clear case names.
- Run `go test ./...` before opening a PR.

## Git Guidelines
- Commit messages in history are short, descriptive, and start with a capital letter (e.g., "Adding standard gitignore.").
- PRs should describe the change, reference any related issue, and include steps to verify.
- Include screenshots only when UI output changes.
