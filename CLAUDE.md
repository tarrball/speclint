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
UPDATE_SCRIPTS=1 go test -run TestScripts ./cmd/specval   # rewrite expected CLI output
```

## Layout

- `cmd/specval/`: the `main` package. `script_test.go` runs the end-to-end tests
  in `testdata/script/*.txtar` (testscript). Each script holds the commands, the
  input files, and the expected output.
- `internal/cli/`: the commands. One file for each command.
- `internal/config/`: finds and reads `.specval.yaml`, and lists the spec, data,
  and policy files.
- `internal/policy/`: the `Engine`. It loads data and Rego rules, evaluates specs
  and the registry, and returns findings.
- `internal/baseline/`: the built-in schema, layer data, and Rego rules (embedded).
  `policy/*_test.rego` are Rego unit tests. `policy_test.go` runs them.
- `internal/yamlpath/`: finds the line of a path such as `calls[2].to` in YAML.
- `internal/claude/`: the Claude Code settings file, hook event, and skill.
- `internal/scaffold/`: the files that `specval init` and `specval new` write.
- `internal/doctor/`: the checks that `specval doctor` runs.
- `examples/refund/`: a sample project. The tests use its specs.
- `docs/`: the user documentation, the style guide, and
  [docs/architecture.md](docs/architecture.md), which tells how `check` works.

`cmd/specval` reads the process state (stdio, working directory, home directory)
into a `cli.Env`. Code in `internal/` gets that state only from its parameters.

## Rules

- Follow [docs/style.md](docs/style.md). All text, including comments, CLI output,
  and Rego `fix` strings, follows ASD-STE100.
- Follow [CONTRIBUTING.md](CONTRIBUTING.md) for branches, commits, and pull requests.
- Do not ignore an error. Do not add a `//nolint` without a linter name and a reason.
- A change in behavior needs a test that fails without it.
- If you change CLI output, update the scripts with `UPDATE_SCRIPTS=1`, then read
  the diff of `testdata/script` line by line. The diff is the change in behavior.
- If you add a built-in rule, add it to the rule tables in docs/specs.md.
  A test fails if a rule ID is not there.

## Contracts that other people depend on

Do not change these without a migration note in the pull request:

- The Rego data paths `data.dpl` (baseline layer data) and `data.project.<key>`
  (project data files). Project policies use them.
- The Rego packages `specval.spec.<name>` (specval runs them for each spec) and
  `specval.registry.<name>` (specval runs them one time for each `check`).
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
  the software on the machine. The doctor tests put a fake `specval` on PATH.
- Rego string interpolation writes an undefined value as `<undefined>`. `sprintf`
  made the whole expression undefined. In a rule, bind each value that can be
  undefined before you interpolate it. `check_undefined.txtar` covers the cases.
- OPA reads a comment line that starts with the word `METADATA` as an annotation.
  Do not start a comment line in a Rego file or template with that word.
- `schema.json` is the one source of the spec format. The SV001 fix text, the kinds
  for `specval new`, and the spec ID pattern all come from it.
