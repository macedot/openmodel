# Repository Guidelines

## Project Structure & Module Organization
`cmd/` contains the CLI entrypoint and top-level tests. Core application code lives under `internal/`: `api/` for OpenAI and Anthropic types/validation, `config/` for loading and hot reload, `provider/` for upstream integrations, `server/` for Fiber handlers, failover, streaming, and rate limiting, plus `logger/`, `endpoints/`, and `state/`. Use `tests/` and package-level `*_test.go` files for integration and behavior coverage. Generated OpenAPI types belong in `internal/api/openai/generated/` and should not be edited manually.

## Build, Test, and Development Commands
Use `make build` to compile `./openmodel`. Run `make run` to build and start the server locally. `make test` executes all Go tests with race detection and coverage output. `make cover` writes `coverage.out` and prints the aggregate percentage. `make check` runs `fmt`, `vet`, and the full test suite. `make lint` runs `golangci-lint` when installed. For focused work, use commands such as `go test -v -run "^TestHandleHealth$" ./internal/server` or `make test-spec` for OpenAI compatibility checks.

## Coding Style & Naming Conventions
Follow standard Go formatting with tabs via `go fmt`; keep imports grouped as standard library, external, then internal packages. File names use `snake_case.go`; tests use `*_test.go`. Exported identifiers use `PascalCase`, internal names use `camelCase`. Prefer explicit error wrapping, for example `fmt.Errorf("load config: %w", err)`. Fiber handlers should accept `*fiber.Ctx`, use `fiber.Status*` constants, and return structured JSON errors.

## Testing Guidelines
Write table-driven tests with named subtests and `testify/assert` or `testify/require`. Name tests as `Test<Function>_<Scenario>` when a scenario suffix adds clarity. Run `make test` before opening a PR and `make cover` when touching core routing, provider, or config logic. Keep mocks in the relevant test file unless they are broadly reused.

## Commit & Pull Request Guidelines
Recent history follows Conventional Commit style such as `feat(provider): ...`, `fix(watcher): ...`, and `docs: ...`; keep using that pattern. Pull requests should describe the behavior change, list validation performed, and link any related issue. Include sample requests, config snippets, or log excerpts when changing API behavior, streaming, hot reload, or benchmark output.

## Security & Configuration Tips
Do not commit real API keys or local config files. Use `${VAR}` expansion in `openmodel.json` and start from `openmodel.json.example`. Treat trace files (`trace-*.json`) and benchmark artifacts (`bench-*.json`) as local debugging output unless a change explicitly requires them.
