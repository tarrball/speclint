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
- [x] Rename the command to `sv`. The product name stays specval (#25).
- [x] Report two services in `services.new` with the same name (SV110).
      Before, a draft with such names stopped the check with an error in `dpl.rego`.

### 2. The baseline layer model

- [x] Add the client and utility layers.
- [x] Add `may_call` and `called_by` to registry entries, with a spec rule (SV111) and
      registry rules (SR005, SR006, SR007).
- [x] Update `sv context`, the docs, and the rule tables.

### 3. Start dogfooding

- [x] Add `.specval/` with a registry of the packages that exist, in their target
      layers. The services that do not exist yet come in the specs of phase 4.
- [x] Add rules that read only the spec text: Given/When/Then, size, kind,
      `out_of_scope` for a feature, and STE (word limits, banned words, title).
- [ ] Each PR from this point has a spec.

### 4. Move the code to the target, one spec for each PR

- [x] Extract `CheckManager` (internal/check). The check and hook commands call only
      CheckManager, and the hook gets the protocol from ClaudeHook.
- [x] Move the baseline to the utility layer as `Baseline`, with
      `services.relayered` (specs/baseline-utility.yaml). It is data in the binary.
- [x] ContextCommand calls a new ContextManager (internal/describe), not ConfigAccessor
      and PolicyEngine.
- [x] NewCommand calls only ScaffoldManager (specs/new-scaffold.yaml).
- [x] Move the file reads out of `policy` (specs/pure-engine.yaml). ConfigAccessor
      reads the files, not a new ProjectDataAccessor: an accessor cannot call
      another accessor, and ConfigAccessor already lists the files.
- [x] Divide the Claude Code parts: ClaudeAccessor (the settings and the skill) and
      ClaudeHook (a utility for the hook protocol). Only the registry changed.
- [ ] Fix the bugs from the review: `settings.json` order and escapes, multi-document specs.
- [ ] Use one table for the commands. Get the kinds and the ID text from the schema.
- [ ] Add a `version` command, signal handling, and a time limit for evaluation.
- [ ] Use path segments inside the code. The JSON output keeps the path strings.
- [ ] Add depguard rules that agree with the registry.
- [ ] Add a test that compares the registry with `go list -deps`. It starts with a
      list of known violations. Each PR makes the list shorter.
- [ ] Make release binaries.

### 5. Brownfield features

Design these from a real brownfield project: the next repository, a project
on Supabase with real tickets. Its layers (for example edge functions and
database functions) and its old code will show what these features must do.

- [ ] A project file that changes the layers. It has the shape of `dpl.yaml`, and the rules see it as `data.dpl`.
- [ ] Turn a rule off, or change its severity, in `.specval.yaml`.
- [ ] A `legacy` layer for a strangler migration:
  - Only an accessor can call a legacy service.
  - A legacy service can call only a manager.
  - A spec cannot add a legacy service.
  - specval warns about a new call between two legacy services.
- [ ] Schema extensions. Then a rule that finds a change to a contract.

## Experiments

We test specval with agents. We record each result, also a bad result, and we
change specval because of it. One run is one sample, so a small difference can
be noise.

### Experiment 1: does a valid spec give a better pull request?

The change: move `run` from `internal/cli/check.go` into a new `CheckManager`.

1. A "blind" agent wrote the spec. It did not know the format, and it used only
   the findings of `sv check --strict`.
2. An "informed" agent wrote the spec with the skill, `sv context`, and the docs.
3. Agent A wrote code from the first draft of the blind agent (not valid).
   Agent B wrote code from the last draft of the blind agent (valid).

| Spec | Runs of `sv check` | Time | Tokens | Tool calls | Calls | Criteria |
|---|---|---|---|---|---|---|
| Blind | 4 | 70 s | 47.7k | 11 | 4 | 2 |
| Informed | 3 | 132 s | 69.3k | 16 | 7 | 5 |

| Code | Time | Tokens | Tool calls | CI | Copilot | Hook finds the configuration |
|---|---|---|---|---|---|---|
| A (spec not valid) | 211 s | 81.9k | 22 | pass | 0 findings | in CheckManager |
| B (spec valid) | 247 s | 82.3k | 33 | pass | 1 high finding | in the hook (a client calls an accessor) |

Results:

- The blind agent made a valid spec in 4 runs. The fixes for missing and
  unknown fields were clear. The fix for a wrong type sent it to `sv schema`.
- **A valid spec can have a gap in the design.** The blind spec did not say
  that the hook must stop the call to the configuration. Agent B followed the
  spec, and the gap became a bug (the Copilot finding).
- The informed spec had the correct design, because the agent read the layer
  rules. The guidance cost more time and tokens.
- Both agents had to use `kind: feature` for a change of structure (PLN003).
  The informed agent found that PLN001 and STE001 together remove detail
  from the criteria.

Changes because of experiment 1:

- [x] PLN003 warns only for a bugfix. STE001 permits 40 words in a criterion.
- [x] The registry records the calls of each service. A spec can remove a call
      (`removed_calls`). specval checks the calls of each changed service
      after the change (SV112, SV202, SR008). With the new rules, the blind
      spec of experiment 1 gets SV202 for the call from HookCommand to
      ConfigAccessor: the gap that became a bug.
- [x] Better fixes: the shape for a wrong type, and the layer of each
      registered service (specs/better-fixes.yaml).
- [x] A note that more rules run after the format errors are fixed
      (specs/format-errors-note.yaml).
- [x] Run experiment 1 again after these changes (experiment 1b, below).

Questions from the work on these changes:

- The hook reads the event of Claude Code with `internal/claude`, so HookCommand
  calls ClaudeAccessor, and SV202 reports it. The event is a format, not stored
  data. Thus a part of `internal/claude` can be a utility, not an accessor.
  Decision: yes. The registry now has `ClaudeHook`, a utility (see 1b).
- `sv context` does not show the calls of each service. A change to it is a
  change to ContextCommand, and ContextCommand calls accessors and the policy
  engine today. Thus such a spec must first move that work to a manager.

### Experiment 1b: call tracking only

The same blind task as experiment 1, with SV202. The fixes did not change, so
the result shows the effect of call tracking only.

| | Spec: runs / time / tokens | Code: time / tokens / tool calls | Copilot | Configuration | Claude Code event |
|---|---|---|---|---|---|
| A | (not valid) | 211 s / 81.9k / 22 | 0 | in the manager | in the hook |
| B | 4 / 70 s / 47.7k | 247 s / 82.3k / 33 | 1 high | in the hook | in the hook |
| 1b | 5 / 129 s / 59.6k | 266 s / 90.9k / 44 | 0 | in the manager | in the manager |

Results:

- **Call tracking closed the gap.** SV202 told the blind agent which calls to
  remove. The commands in 1b import only the new manager, and a test checks it.
- **SV202 forced a decision that the registry made wrong.** The registry had the
  hook protocol in ClaudeAccessor. Thus the agent moved the reading and writing
  of the event into CheckManager. The manager got stdin, stdout, and stderr,
  and two `//nolint:nilerr` lines. The spec showed this
  (`CheckManager -> ClaudeAccessor ReadEvent`), but no rule found it.
- specval does not make an architecture. If the registry is wrong, specval makes
  the code follow the wrong design. Upfront design and a review of the spec stay
  necessary. A team also needs a reviewed way to change the architecture.
- The fixes of SV202 did not tell which services the caller can use, or where
  `removed_calls` goes.

Changes because of experiment 1b:

- [x] The registry has `ClaudeHook`, a utility for the hook protocol.
      ClaudeAccessor keeps the settings and the skill.
- [x] SV202: name the layers that the caller can call, and tell that
      `removed_calls` is a top-level field (specs/better-fixes.yaml).
- [x] A spec can change the architecture: `services.relayered: [{ name, layer,
      rename, reason }]`. specval checks the spec with the registry after the
      move (SV113 to SV117, SV203). The fix of SV202 offers the move, which a
      reviewer sees in the spec (specs/relayered.yaml).

### The next experiments

The experiments in this repository have two problems:

- The agents are not blind: the task is about specval, and the code and
  CLAUDE.md tell about specs and layers.
- specval is a CLI in the DPL layers. The DPL layers are for services.

Copilot is also not a controlled measure: it reads the whole repository, and
its result can change from one run to the next.

When specval works well for its own code, we move the experiments:

- [ ] Make a separate repository with a small DPL service: HTTP endpoints as
      clients, managers, engines, accessors, and a database as a resource. Later,
      add an old version with layer violations for the brownfield features.
- [ ] Measure with our own checks: the tests pass, the imports agree with the
      registry after the change, and the code has each call of the spec and no
      other calls. Also record the time, tokens, tool calls, and diff size.
- [ ] Use a reviewer agent with a fixed list of checks. It sees only the diff and
      the target architecture. Copilot is a second opinion only.
- [ ] Run each arm 3 or more times.

## Ideas for later

- Warn when a client calls more than one manager. This can show a problem in
  the decomposition, because a manager does the orchestration.
- A built-in rule: the file name of a spec is its `id`. Today `sv new` writes
  `<id>.yaml`, but nothing finds a file name and an `id` that do not agree.
  The rules see only the content of the spec. With this rule, a project rule on
  `input.id` also controls the file names, so specval needs no other feature
  for a naming convention. The convention is the choice of each project, for
  example a ticket key (`PROJ-123-short-name`) or a date.
- A fix for a comma in a flow mapping: `{ purpose: A, B, and C }` makes YAML
  read `B` and `and C` as keys, and SV001 says "Remove `and`". If a key has
  a space, the fix can tell to quote the value.
- For the specs of specval: a number prefix (`0003-extract-check-manager`),
  because the specs build on each other, and the number gives their order.
