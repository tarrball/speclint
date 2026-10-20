# History

This file records the work that made speclint what it is now, and the results
of the first experiments. The next work is in [ROADMAP.md](../ROADMAP.md).

This file uses the old names. Before v0.4.0, the product was specval, the
command was sv, and the project directory was `.specval`.

The first plan joined two tasks. specval checked its own specs (dogfooding),
and its code moved to the DPL layers. specval was a brownfield project: the
code existed, and it did not fit the layers. Each PR moved it nearer to the
layers. At the end of phase 4, the registry and the imports agreed, with no
layer violations.

## Decisions of the first plan

- **Spec lifecycle.** A spec has an optional `status: draft | done`. A spec
  without a status is a draft. specval runs only SV001 on a `done` spec. The PR
  that implements a spec registers its services and sets `done` in the same commit.
- **Layers.** specval is for the DPL architecture now. Later, a project can
  change the layers, for example a brownfield project or a strangler migration.
- **specval uses the DPL layers itself**, with the client and utility layers.
- **Suffixes.** The client and utility layers have no suffix. The other suffixes
  do not change.

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
- [x] Each PR from this point has a spec. CONTRIBUTING.md now makes this a rule.

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
- [x] Keep `settings.json` as it is when init adds the hook (specs/settings-json.yaml).
- [x] Report a spec or data file with more than one YAML document
      (specs/multi-document.yaml).
- [x] Use one table for the commands. Get the kinds from the schema, and use one
      ID rule text (specs/command-table.yaml).
- [x] Add a `version` command, signal handling, and a time limit for evaluation
      (specs/version-signals-timeout.yaml).
- [x] Read paths into segments (yamlpath), and quote a key that is not plain
      (`lib.path_key`). The paths stay strings (specs/path-segments.yaml).
- [x] Add a test that compares the registry with the imports of the packages
      (TestRegistryAgreesWithTheImports). The list of known violations is empty,
      because the registry and the imports agree.
- Not done: depguard rules. They would be a second copy of the registry, which
  can disagree with it. The test reads the registry, so it cannot.
- [x] Make release binaries: `mise run dist -- <tag>` and a workflow on each `v*` tag.

### 4b. Agent use (before v0.1.0)

From a review of how well an agent can use sv:

- [x] A change inside one service has no calls. The services in
      services.changed then cover the acceptance criteria (SV118).
- [x] Give each command a help text with a description and examples, on stdout
      (specs/command-help.yaml).
- [x] `sv init` writes Copilot review instructions that compare a pull request
      with its spec, and prints a line to add to CLAUDE.md. doctor checks them
      (specs/copilot-review.yaml).
- [x] `sv init --dry-run`. Do not add `project.rego` to a policy directory that
      already has a `.rego` file (specs/init-dry-run.yaml).
- [x] The skill: run `sv check` when the hook did not run, and after the
      registry update. `sv new` prints a relative path (specs/skill-text.yaml).

### 4c. Rename and setup options (v0.4.0)

- [x] Rename the product to speclint, the command to slint, and the repository
      to tarrball/speclint, with no migration path for the old names (#70).
- [x] `slint init` adds the Claude Code skill and hook only with `--claude`.
      doctor checks them only if the project or the user has one of them
      (#71, #72).

## Experiments

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
