# specval (prototype)

Validate a feature/bugfix spec against architecture rules **before any code is written**.
A single Go binary with OPA embedded: no conftest, Docker, or runtime to install.

```
specval init                 # .specval.yaml, registry, project policy, Claude skill + hook
specval new ORD-150 --title "Customer can cancel an order"
specval check                # every spec in specs/; exit 1 on errors (--format json, --strict)
specval context              # spec format + layers + registry + rules, for an agent (~750 tokens)
specval schema               # the spec JSON Schema
specval hook                 # Claude Code PostToolUse hook (JSON on stdin)
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

`specval init` writes `.claude/skills/spec/SKILL.md` (tells Claude to run
`specval context`, write the spec, and fix findings) and a PostToolUse hook in
`.claude/settings.json`. When Claude writes a file under the specs dir, the hook
runs the check; errors go back to Claude (exit 2) with their fixes, and warnings
are passed along as context.

## Dev

```
mise exec go@1.25.12 -- go test ./...
mise exec go@1.25.12 -- go build -ldflags "-s -w" -o bin/specval .
```

`examples/refund` has one passing spec (ORD-142), one with architecture
violations (ORD-143), and one with schema errors (ORD-144).
