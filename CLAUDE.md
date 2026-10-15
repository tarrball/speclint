# CLAUDE.md

specval is a Go CLI. It validates a change spec (a YAML file) against architecture
rules before a developer writes code. The rules are Rego policies. The binary
contains the OPA library, which evaluates them.

## Commands

```
mise run check   # lint and test; must pass before each commit
mise run fmt     # format Go and Rego
mise run build   # build bin/specval
go test -run TestName ./...
```

## Layout

All Go code is in package `main` at the repository root.
A later change moves it to `cmd/specval` and `internal/` (see docs/style.md).

- `main.go`: command dispatch, `check`, `context`, `new`, `hook`.
- `engine.go`: loads data and policies, evaluates specs, maps findings to YAML lines.
- `setup.go`: `init`, Claude Code settings, `doctor`.
- `config.go`: finds and reads `.specval.yaml`.
- `baseline/`: the built-in schema, layer data, and Rego rules (embedded).
- `templates/`: the files that `specval init` writes (embedded).
- `examples/refund/`: a sample project. The tests use its specs.

## Rules

- Follow [docs/style.md](docs/style.md). All text, including comments, CLI output,
  and Rego `fix` strings, follows ASD-STE100.
- Follow [CONTRIBUTING.md](CONTRIBUTING.md) for branches, commits, and pull requests.
- Do not ignore an error. Do not add a `//nolint` without a linter name and a reason.
- A change in behavior needs a test that fails without it.

## Contracts that other people depend on

Do not change these without a migration note in the pull request:

- The Rego data paths `data.dpl` (baseline layer data) and `data.project.<key>`
  (project data files). Project policies use them.
- The Rego packages `specval.spec.<name>` (run once for each spec) and
  `specval.registry.<name>` (run once for each `check`).
- Rule IDs. `SV000` is YAML syntax, `SV001` is the schema, `SV1xx` are spec errors,
  `SV2xx` are spec warnings, and `SR0xx` are registry rules.
- The JSON output of `check --format json` and `doctor --format json`.
- The exit codes: 0 pass, 1 findings, 2 usage or internal error.
  `specval hook` uses Claude Code's codes: 2 sends stderr to Claude.

## Things to know

- `//go:embed` cannot reach a parent directory. The embedded directories must stay
  inside the package that embeds them.
- The opa CLI version in `mise.toml` must agree with the OPA module version in `go.mod`.
- `specval doctor` runs the `specval` binary on PATH. Its tests must not depend on
  the software on the machine.
