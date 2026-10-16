# Plan: dogfooding and architecture cleanup

This file is temporary. Remove it when the plan is complete.

This plan joins two tasks. specval checks its own specs (dogfooding), and the
code moves to the DPL layers. specval is a brownfield project: the code exists,
and it does not fit the layers yet. Each PR moves it nearer to the layers. Thus
specval tests the brownfield features before other projects use them.

## Decisions

- **Spec lifecycle.** A spec has an optional `status: draft | done`. A spec
  without a status is a draft. specval runs only SV001 on a `done` spec. The PR
  that implements a spec registers its services and sets `done` in the same commit.
- **Layers.** specval is for the DPL architecture now. Later, a project can
  change the layers, for example a brownfield project or a strangler migration.
- **specval uses the DPL layers itself**, with the client and utility layers.
- **Suffixes.** The client and utility layers have no suffix. The other suffixes
  do not change.

## The layer model

| Layer | Can call |
|---|---|
| client | manager, utility |
| manager | engine, accessor, utility |
| engine | accessor, utility |
| accessor | resource, utility |
| resource | utility |
| utility | utility |

A registry entry can make the layer rules stricter. It cannot make them wider.

```yaml
services:
  ConfigUtility: { layer: utility, may_call: [] }
  AuditUtility:  { layer: utility, called_by: [manager] }
```

A value in `may_call` or `called_by` is a layer or a service. A built-in rule
checks each call against the layer rules and against the entries of the caller
and the callee. A registry rule warns if an entry permits a call that the layer
rules do not permit.

## specval in the layers (the target)

| Layer | specval |
|---|---|
| client | the commands: check, hook, doctor, init, new, context, schema |
| manager | `CheckManager` (new; the `run` function from `cli/check.go`), `DoctorManager`, `SetupManager` |
| engine | `PolicyEngine` (evaluation only), `ScaffoldEngine` |
| accessor | `ConfigAccessor`, `ProjectDataAccessor` (new; file reads from `policy`), `ClaudeAccessor`, `BaselineAccessor` |
| utility | `YamlPathUtility` |
| resource | the file system, the process |

## Phases

### 0. Make the rule interface stable, and close the gate

- [x] Remove the nondeterministic Rego built-ins, such as `http.send` and
      `time.now_ns`. Project rules cannot use the schema functions (#20).
- [x] Add `data.specval.lib` to the contracts in CLAUDE.md (#20).
- [x] The hook reports a configuration that is not valid. It does not pass silently (#19).
- [x] Add CI: `mise run check` and govulncheck (#21).

### 1. Spec lifecycle

- [x] Add `status: draft | done` to the schema. `sv new` writes `draft` (#23).
- [x] specval runs only SV001 on a `done` spec. Go code makes this decision, not Rego (#23).
- [x] Cache the Go build in CI (#24).
- [x] Rename the command to `sv`. The product name stays specval.
- [ ] Report two services in `services.new` with the same name. Now a draft
      with such names stops the check with an error in `dpl.rego`.

### 2. The baseline layer model

- [ ] Add the client and utility layers.
- [ ] Add `may_call` and `called_by` to registry entries, with a spec rule and a registry rule.
- [ ] Update `sv context`, the docs, and the rule tables.

### 3. Start dogfooding

- [ ] Add `.specval/` with the target registry above.
- [ ] Add rules that read only the spec text: Given/When/Then, size, kind,
      `out_of_scope` for a feature, and STE (word limits, banned words, title).
- [ ] Each PR from this point has a spec.

### 4. Move the code to the target, one spec for each PR

- [ ] Extract `CheckManager`. Move the file reads from `policy` to `ProjectDataAccessor`.
- [ ] Put the Claude Code parts behind an accessor.
- [ ] Fix the bugs from the review: `settings.json` order and escapes, multi-document specs.
- [ ] Use one table for the commands. Get the kinds and the ID text from the schema.
- [ ] Add a `version` command, signal handling, and a time limit for evaluation.
- [ ] Use path segments inside the code. The JSON output keeps the path strings.
- [ ] Add depguard rules that agree with the registry.
- [ ] Add a test that compares the registry with `go list -deps`. It starts with a
      list of known violations. Each PR makes the list shorter.
- [ ] Make release binaries.

### 5. Brownfield features

Design these from the results of phase 4.

- [ ] A project file that changes the layers. It has the shape of `dpl.yaml`, and the rules see it as `data.dpl`.
- [ ] Turn a rule off, or change its severity, in `.specval.yaml`.
- [ ] A `legacy` layer for a strangler migration:
  - Only an accessor can call a legacy service.
  - A legacy service can call only a manager.
  - A spec cannot add a legacy service.
  - specval warns about a new call between two legacy services.
- [ ] Schema extensions. Then a rule that finds a change to a contract.

## Ideas for later

- Warn when a client calls more than one manager. This can show a problem in
  the decomposition, because a manager does the orchestration.
