# Contributing

## Set up

1. Install [mise](https://mise.jdx.dev/).
2. In the repository root, run `mise install`.
   This installs the pinned versions of Go, golangci-lint, the opa CLI, and Regal.
3. Run `mise run check` to make sure that lint and tests pass.

## Tasks

| Command | What it does |
|---|---|
| `mise run check` | Runs lint and tests. Do this before you push. |
| `mise run test` | Runs all tests. |
| `mise run lint` | Checks Go and Rego code. |
| `mise run fmt` | Formats Go and Rego code. |
| `mise run cover` | Runs all tests and opens the coverage report. |
| `mise run build` | Builds `bin/slint`. |
| `mise run specs` | Checks the specs in `specs/` and the registry with speclint itself. |
| `mise run vuln` | Finds known vulnerabilities in the code and its dependencies. It needs the network. |

CI runs `mise run check` and `mise run vuln` for each pull request and each
push to `main`. A pull request must pass CI before you merge it.

## Rules for changes

Before you write code, comments, or documentation, read [docs/style.md](docs/style.md).

- After each change, `mise run check` must pass.
- A change in behavior must have a test that fails before the change and passes after it.
- The end-to-end tests in `testdata/script` hold the exact CLI output.
  If you change the output, run `UPDATE_SCRIPTS=1 go test -run TestScripts ./cmd/slint`.
  Then examine the diff. Include it in the same commit.
- A change to the CLI output, a rule, or a configuration file must also update the documentation.
- If you change the version of `github.com/open-policy-agent/opa` in `go.mod`,
  change the `opa` version in `mise.toml` to the same version.

## Specs

speclint checks its own changes (dogfooding). The registry of speclint is
`.speclint/data/services.yaml`. It puts each package in a DPL layer.

- If a change adds or changes Go code under `cmd/` or `internal/`, write a spec
  first: `specs/<id>.yaml`. Use the topic of the branch as the ID, for example
  `extract-check-manager`.
- Make the spec pass before you write the code. The Claude Code skill and hook
  help with this.
- In the same pull request, update the registry as the spec tells (see "The
  life of a spec" in docs/specs.md), and set `status: done` in the spec.
- A change to only documentation, CI, tests, or Rego rules does not need a spec,
  because it has no calls between services.
- `TestRegistryAgreesWithTheImports` (cmd/slint) compares the registry with the
  imports of the Go packages. If you add an import between two packages, add
  the call to the registry in the same pull request.

## Branches and pull requests

1. Make a branch from `main`. Name it `<type>/<topic>`, for example `fix/hook-relative-path`.
   Use one of these types: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`.
2. Keep each pull request to one topic. Make it small enough to review in one session.
   If a change is large, divide it into a sequence of pull requests.
3. In the description, tell why you made the change and how you tested it.
4. Request a review from Copilot: `gh pr edit --add-reviewer @copilot`.
5. Read each review comment. Fix the problem, or reply with the reason that you did not.
6. When the review is complete, squash and merge.
