# CLAUDE.md

speclint is a Go CLI. It validates a change spec (a YAML file) against architecture
rules before a developer writes code. The rules are Rego policies. The binary
contains the OPA library, which evaluates them.

## Commands

```
mise run check   # lint, test, and check specs/; must pass before each commit
mise run fmt     # format Go and Rego
mise run build   # build bin/slint
go test -run TestName ./...
UPDATE_SCRIPTS=1 go test -run TestScripts ./cmd/slint   # rewrite expected CLI output
```

## Layout

- `cmd/slint/`: the `main` package. `script_test.go` runs the end-to-end tests
  in `testdata/script/*.txtar` (testscript). Each script holds the commands, the
  input files, and the expected output.
- `internal/cli/`: the commands. One file for each command.
- `internal/check/`: CheckManager. It finds the specs, runs the `Engine`, and
  makes the report for `check` and `hook`.
- `internal/describe/`: ContextManager. It gives the layers, the registry, and
  the rules for `context`.
- `internal/config/`: finds and reads `.speclint.yaml`, and lists and reads the
  spec, data, and policy files.
- `internal/policy/`: the `Engine`. It loads data and Rego rules, evaluates specs
  and the registry, and returns findings. It reads no files: the managers give
  it the content.
- `internal/baseline/`: the built-in schema, layer data, and Rego rules (embedded).
  `policy/*_test.rego` are Rego unit tests. `policy_test.go` runs them.
- `internal/yamlpath/`: finds the line of a path such as `calls[2].to` in YAML.
- `internal/claude/`: the Claude Code settings file, hook event, and skill.
- `internal/copilot/`: the Copilot review instructions that `slint init` writes.
- `internal/scaffold/`: the files that `slint init` and `slint new` write.
- `internal/doctor/`: the checks that `slint doctor` runs.
- `.speclint/`, `specs/`: the configuration, registry, and specs of speclint itself
  (dogfooding).
- `examples/refund/`: a sample project. The tests use its specs.
- `docs/`: the user documentation, the style guide, and
  [docs/architecture.md](docs/architecture.md), which tells how `check` works.

`cmd/slint` reads the process state (stdio, working directory, home directory)
into a `cli.Env`. Code in `internal/` does not read stdio, the working directory,
or the home directory of the process; it gets them from its parameters. One
exception: doctor reads PATH, because the hook runs the `slint` on PATH.

## Rules

- Follow [docs/style.md](docs/style.md). All text, including comments, CLI output,
  and Rego `fix` strings, follows ASD-STE100.
- Follow [CONTRIBUTING.md](CONTRIBUTING.md) for branches, commits, and pull requests.
- Before you change Go code under `cmd/` or `internal/`, write a spec in `specs/`
  that passes `slint check` (see CONTRIBUTING.md, "Specs").
- The calls in `.speclint/data/services.yaml` agree with the imports between the
  packages. `TestRegistryAgreesWithTheImports` fails if they do not.
- Do not ignore an error. Do not add a `//nolint` without a linter name and a reason.
- A change in behavior needs a test that fails without it.
- If you change CLI output, update the scripts with `UPDATE_SCRIPTS=1`. Then read
  the diff of `testdata/script` line by line. The diff is the change in behavior.
- If you add a built-in rule, add it to the rule tables in docs/specs.md.
  A test fails if a rule ID is not there.

## Contracts that other people depend on

Do not change these without a migration note in the pull request:

- The Rego data paths `data.dpl` (baseline layer data) and `data.project.<key>`
  (project data files). Project policies use them.
- The Rego packages `speclint.spec.<name>` (speclint runs them for each spec) and
  `speclint.registry.<name>` (speclint runs them one time for each `check`).
- The functions in `data.speclint.lib`, for example
  `lib.finding(meta, path, msg, fix)`. Project rules call them. `path` stays a string.
- The built-in functions that a rule can use. speclint removes the
  nondeterministic ones, and project rules cannot use the schema functions.
  Do not remove more without a migration note.
- Rule IDs. `SV000` is YAML syntax, `SV001` is the schema, `SV1xx` are spec errors,
  `SV2xx` are spec warnings, and `SR0xx` are registry rules.
- The JSON output of `check --format json` and `doctor --format json`.
- The exit codes: 0 pass, 1 findings, 2 usage or internal error.
  `slint hook` uses Claude Code's codes: 2 sends stderr to Claude.

## Things to know

- `//go:embed` cannot reach a parent directory. The embedded directories must stay
  inside the package that embeds them.
- The opa CLI version in `mise.toml` must agree with the OPA module version in `go.mod`.
- `slint doctor` runs the `slint` binary on PATH. Its tests must not depend on
  the software on the machine. The doctor tests put a fake `slint` on PATH.
- Rego string interpolation writes an undefined value as `<undefined>`. `sprintf`
  makes the whole expression undefined. In a rule, bind each value that can be
  undefined before you interpolate it. `check_undefined.txtar` covers the cases.
- OPA reads a comment line that starts with the word `METADATA` as an annotation.
  Do not start a comment line in a Rego file or template with that word.
- `schema.json` is the one source of the spec format. The SV001 fix text, the kinds
  for `slint new`, and the spec ID pattern all come from it.
