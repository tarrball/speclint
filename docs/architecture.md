# Architecture

This document tells how specval works inside. Read it before you change the
code. For the rules of the code, see [style.md](style.md).

## Packages

| Package | Purpose |
|---|---|
| `cmd/specval` | The `main` package. It reads the process state into a `cli.Env` and calls `cli.Run`. It also holds the end-to-end tests. |
| `internal/cli` | The commands. Each command has its own file. The commands get their input and output only from `cli.Env`. |
| `internal/config` | Finds `.specval.yaml` and lists the spec, data, and policy files. |
| `internal/policy` | The `Engine`. It loads the data and the rules, evaluates them, and returns findings. |
| `internal/baseline` | The built-in schema, layer data, format example, and Rego rules. The binary contains them. |
| `internal/yamlpath` | Finds the line of a path such as `calls[2].to` in a YAML document. |
| `internal/claude` | Reads and changes the Claude Code settings file, reads the hook event, and holds the skill. |
| `internal/scaffold` | Writes the files of `specval init` and `specval new`. |
| `internal/doctor` | The checks of `specval doctor`. |

The packages depend on each other in this direction:

```
cmd/specval ─> cli ─┬─> policy ──> baseline, config, yamlpath
                    ├─> doctor ──> policy, claude, config
                    ├─> scaffold ─> baseline, claude, config
                    └─> claude, config, baseline
```

## How `specval check` works

1. `config.Find` reads `.specval.yaml` from the directory of the first spec,
   or from a parent directory. All specs must be in the same project.
2. `policy.New` makes an `Engine`:
   1. `baseline.Data` makes `data.dpl`. It holds the layer rules from
      `dpl.yaml` and the schema. It also holds two tables that the schema
      rule uses to write fixes: the permitted keys at each path, and the
      description of each path.
   2. The engine loads each project data file. Each top-level key becomes
      `data.project.<key>`. The engine keeps the YAML nodes of each file, so
      that it can find the line of a registry finding.
   3. The engine parses the baseline rules and the project rules. It refuses a
      `deny` or `warn` rule that has no `custom.id`.
   4. The engine prepares two queries: one for the spec rules, one for the
      registry rules.
3. `Engine.CheckRegistry` runs the registry rules one time. It gives each
   finding the data file and the line of its path.
4. `Engine.CheckSpec` runs for each spec:
   1. It parses the YAML. A syntax error becomes an SV000 finding.
   2. It evaluates the spec rules with the spec as `input`. If the schema rule
      (SV001) finds an error, it returns only the schema errors.
   3. It finds the line of each finding with `yamlpath.Line`, and sorts the
      findings by line and rule ID.
5. `cli` writes the report as text or JSON and sets the exit status.

`specval hook` uses the same steps for one spec. `specval context` uses the
engine to list the layers, the registry, and the rules.

## Design decisions

- **The binary contains the rules.** specval embeds the OPA library and the
  baseline rules. A project needs only the binary.
- **Rules describe themselves.** Each rule has a `METADATA` block with an ID
  and a title. `lib.finding` reads them, and `specval context` lists them.
  Thus the findings and the rule list use the same text.
- **The schema comes first.** If a spec has the wrong shape, the output of the
  other rules does not help, so specval shows only the schema errors.
- **Each finding has a fix.** The reader is often an agent. A fix that is an
  instruction lets the agent correct the spec without other help.
- **The schema is the one source of the spec format.** The fix text of SV001,
  the list of kinds for `specval new`, and the pattern of a spec ID all come
  from `schema.json`.

## Tests

| Kind | Where | What it covers |
|---|---|---|
| Unit tests | `internal/*/*_test.go` | Each package, through its API. |
| Rego tests | `internal/baseline/policy/*_test.rego` | Each baseline rule. `TestRegoPolicies` runs them with the real `data.dpl`. |
| End-to-end tests | `cmd/specval/testdata/script/*.txtar` | Each command, with its exact output and exit status. |

To update the expected output of the end-to-end tests, run
`UPDATE_SCRIPTS=1 go test -run TestScripts ./cmd/specval`. Then examine the
diff: it shows each change in behavior.
