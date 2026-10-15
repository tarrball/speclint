# specval (prototype)

Validate a feature/bugfix spec against architecture rules **before any code is written**.
A single Go binary with OPA embedded: no conftest, Docker, or runtime to install.

```
specval init                 # .specval.yaml, registry, project policy, Claude skill + hook (re-runnable)
specval doctor               # is the setup still intact? (--format json)
specval new ORD-150 --title "Customer can cancel an order"
specval check                # every spec in specs/; exit 1 on errors (--format json, --strict)
specval context              # spec format + layers + registry + rules, for an agent (~750 tokens)
specval schema               # the spec JSON Schema
specval hook                 # run by Claude Code after each file write; checks the spec it just saved (see below)
```

## How rules are layered

| Where | What | Who owns it |
|---|---|---|
| Embedded in the binary (`baseline/`) | spec schema, DPL layer rules (`dpl.yaml` → `data.dpl`), SV/SR rules | DPL |
| `.specval/data/*.yaml` | service registry and other project data (→ `data.project.<key>`) | project (CODEOWNERS) |
| `.specval/policy/*.rego` | project rules, packages `specval.spec.<name>` (per spec) or `specval.registry.<name>` (once, against data) | project |

The schema check (SV001) runs first, and other rules run only once it passes.
Registry rules (SR*) run once per `check` and report against the registry file, not against each spec.

## Writing a rule

Every `deny` (error) or `warn` rule needs a METADATA block with `title` and
`custom.id`; the loader refuses rules without one. Build results with `lib.finding`,
which reads the id and title from that metadata, so `specval context` and the
lint output come from the same source:

```rego
# METADATA
# title: Only OrderManager calls PricingEngine
# description: Pricing is orchestrated in one place.
# custom:
#   id: ORD001
deny contains f if {
	some i, c in input.calls
	c.to == "PricingEngine"
	c.from != "OrderManager"
	f := lib.finding(rego.metadata.rule(), sprintf("calls[%d]", [i]),
		sprintf("%s may not call PricingEngine", [c.from]),
		"Have OrderManager call PricingEngine and pass the price to whoever needs it.")
}
```

`path` is a YAML path (`calls[2].to`); specval resolves it to a line number.

## Claude Code

`specval init` sets up two things:

- **A skill** (`.claude/skills/spec/SKILL.md`) that tells Claude, while
  planning, to run `specval context`, write the spec, and fix every finding
  before writing code.
- **A hook** in `.claude/settings.json` that makes Claude Code run
  `specval hook` after every Write, Edit, or MultiEdit.

### What `specval hook` does

Claude Code pipes a JSON event describing the tool call to its stdin.
`specval hook` reads the edited file's path from it, then:

1. **Ignores anything that isn't a spec.** If the file isn't `.yaml`/`.yml`,
   isn't under a `.specval.yaml` project, or isn't in that project's
   `specs_dir`, it exits 0 silently (~20ms, Claude sees nothing).
2. **Checks the spec,** exactly like `specval check <file>`.
3. **Reports back through its exit code:**

| Result | Exit | Effect |
|---|---|---|
| Errors | 2 | Findings and their `fix:` lines go to stderr, which Claude Code hands to Claude, along with an instruction to fix the spec before writing code. |
| Warnings only | 0 | Warnings are passed to Claude as extra context; nothing is blocked. |
| Passes | 0 | Nothing. |
| specval itself fails (e.g. a broken policy) | 1 | Claude Code shows the error to the user, and Claude carries on. |

It's feedback, not a gate. The hook runs after the file is saved, so a failing
spec stays on disk, and nothing *forces* Claude to stop before writing code;
the message and the skill tell it to. A hard gate (a PreToolUse hook that blocks
writes outside `specs/`, or a Stop hook that won't let Claude finish while a
spec fails) is possible, and worth A/B testing against this.

## Keeping the setup healthy

`specval doctor` checks each piece and gives a fix for each problem (exit 1 on any failure):

| Check | Fails when |
|---|---|
| config | no `.specval.yaml` here or in a parent |
| specs / data / policy | a configured directory is missing (warning) |
| rules | a data file or Rego policy won't load, or a rule has no `custom.id` |
| registry | no services registered (warning), or registry rules (SR*) fail |
| skill | `.claude/skills/spec/SKILL.md` is missing or no longer runs `specval context` (warning) |
| hook | no PostToolUse hook runs `specval hook` in project, local, or user settings; `disableAllHooks` is set; a settings file is invalid JSON; the matcher misses Write/Edit/MultiEdit (warning) |
| path | `specval` isn't on PATH or won't run, so the hook can't either; or PATH points to a different binary than the one you ran (warning) |

`specval init` is the repair step. It only creates what's missing and merges the
hook into an existing `.claude/settings.json` without dropping other settings
(the file is rewritten with sorted keys).

## Dev

The tool versions are pinned in `mise.toml`; run `mise install` once to get them.

```
mise run check   # lint and test
mise run build   # build bin/specval
```

`examples/refund` has one passing spec (ORD-142), one with architecture
violations (ORD-143), and one with schema errors (ORD-144).
