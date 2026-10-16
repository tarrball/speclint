# specval

specval checks a change spec against architecture rules before you write code.

A spec is a short YAML file for one ticket. It gives the acceptance criteria,
the services that the change adds or changes, and each call between services.
specval checks the spec against a layered architecture and against the rules
of your project. Each finding tells you how to fix it.

```
$ specval check specs/ORD-143.yaml
specs/ORD-143.yaml:8 error SV107 No call covers AC2 (acceptance[1])
    fix: Add AC2 to `covers` on the call(s) that implement it, or add the missing call.
specs/ORD-143.yaml:10 error SV109 ShippingManager is not a registered service (services.changed[1])
    fix: If ShippingManager is new, move it to services.new with a layer and purpose. Otherwise use a registered service: OrderManager.
specs/ORD-143.yaml:14 error ORD001 OrderAccessor cannot call PricingEngine (calls[2])
    fix: Make OrderManager call PricingEngine, and give the price to the services that need it.
specs/ORD-143.yaml:14 error SV102 OrderAccessor (accessor) cannot call PricingEngine (engine) (calls[2])
    fix: Accessors can call only resources. Send the call to PricingEngine through a manager.
specs/ORD-143.yaml:15 error SV101 RefundEngine is not a registered service (calls[3].to)
    fix: If RefundEngine is new, declare it under services.new with a layer and purpose. Otherwise use a registered service: PricingEngine.
specs/ORD-143.yaml:15 error SV108 AC4 is not an acceptance criterion in this spec (calls[3].covers[1])
    fix: Use one of: AC1, AC2, or add AC4 under acceptance.
specval checked 1 spec(s) and found 6 error(s) and 0 warning(s).
```

specval works with [Claude Code](https://claude.com/claude-code). A skill tells
Claude to write the spec before the code. A hook checks the spec each time
Claude saves it, and sends the findings back to Claude.

## Install

specval needs Go 1.26 or later.

```
go install github.com/tarrball/specval/cmd/specval@latest
```

Make sure that the Go bin directory is on your `PATH`. The Claude Code hook
runs the `specval` that is on `PATH`.

## Quick start

1. In the root of your repository, set up specval:

   ```
   specval init
   ```

   This writes `.specval.yaml`, the service registry, a file for project rules,
   the Claude Code skill, and the hook.

2. Add your services and their layers to `.specval/data/services.yaml`:

   ```yaml
   services:
     OrderManager: { layer: manager }
     PricingEngine: { layer: engine }
     OrderAccessor: { layer: accessor }
   ```

3. Make a spec for a ticket, then write its content:

   ```
   specval new ORD-150 --title "Customer can cancel an order"
   ```

4. Check the spec:

   ```
   specval check
   ```

5. When you implement the change, add its new services to the registry.
   In the same change, set `status: done` in the spec. specval then checks
   only the format of the spec.

6. Make sure that the setup works:

   ```
   specval doctor
   ```

For a complete project, see [`examples/refund`](examples/refund). It has one
spec that passes, one spec that breaks the layer rules, and one spec that
breaks the schema.

## Commands

| Command | What it does |
|---|---|
| `specval check [files...]` | Checks the given specs, or each spec in the specs directory. Use `--format json` for JSON output, and `--strict` to fail on warnings. |
| `specval new <id>` | Makes `<specs dir>/<id>.yaml` from the template. Use `--kind` and `--title` to set those fields. |
| `specval context` | Prints the spec format, the layers, the registry, and the rules. An agent reads this text before it writes a spec. |
| `specval schema` | Prints the JSON Schema of a spec. |
| `specval init` | Sets up specval, the skill, and the hook. You can run it again to restore a missing part. |
| `specval doctor` | Checks the setup, and gives a fix for each failure and warning. Use `--format json` for JSON output. |
| `specval hook` | Claude Code runs this command after each file write. Do not run it yourself. |

| Exit status | Meaning |
|---|---|
| 0 | The command succeeded and found no errors. |
| 1 | `check` found an error (or a warning, with `--strict`), or `doctor` found a failure. |
| 2 | The command was not correct, or specval could not do it. |

## Documentation

- [docs/specs.md](docs/specs.md): the spec format and the built-in rules.
- [docs/rules.md](docs/rules.md): how to add project data and project rules.
- [docs/claude-code.md](docs/claude-code.md): the skill, the hook, and `specval doctor`.
- [docs/architecture.md](docs/architecture.md): how specval works inside.
- [CONTRIBUTING.md](CONTRIBUTING.md): how to build, test, and change specval.
