# specval

specval checks a change spec against architecture rules before you write code.

A spec is a short YAML file for one ticket. It gives the acceptance criteria,
the services that the change adds or changes, and each call between services.
specval checks the spec against a layered architecture and against the rules
of your project. Each finding tells you how to fix it.

```
$ sv check specs/ORD-143.yaml
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

On Linux or macOS, run:

```
curl -fsSL https://raw.githubusercontent.com/tarrball/specval/main/scripts/install.sh | sh
```

The script downloads the latest release for your system, compares it with
`checksums.txt`, and puts `sv` in `~/.local/bin`. It does not use sudo. If
that directory is not on `PATH`, the script tells you the line to add to your
shell profile. To update `sv`, run the command again.

To install a given version, set `SV_VERSION` for `sh`:

```
curl -fsSL https://raw.githubusercontent.com/tarrball/specval/main/scripts/install.sh | SV_VERSION=v0.1.0 sh
```

To use another directory, also set `SV_INSTALL_DIR`. Use a directory that is
on your `PATH`.

Or download the binary for your system from the
[releases](https://github.com/tarrball/specval/releases), and put it in a
directory on your `PATH`, for example `~/.local/bin`, with the name `sv`
(`sv.exe` on Windows). `checksums.txt` has the
SHA-256 checksum of each binary. On Linux and macOS, a download does not keep
the permission to run the file. Give the permission before you use `sv`:

```
mv sv_v0.1.0_darwin_arm64 sv
chmod +x sv
```

The binaries are not notarized. If macOS stops `sv` from running, remove the
quarantine attribute: `xattr -d com.apple.quarantine sv`.

Or, with Go 1.26 or later:

```
go install github.com/tarrball/specval/cmd/sv@latest
```

Make sure that the Go bin directory is on your `PATH`.

The Claude Code hook runs the `sv` that is on the `PATH` of Claude Code.
Claude Code reads your shell profile when it starts. After you change `PATH`,
start Claude Code again. Then ask Claude to run `sv doctor`. The path check
shows the `sv` that the hook runs and its version.

## Quick start

1. In the root of your repository, set up specval:

   ```
   sv init
   ```

   This writes `.specval.yaml`, the service registry, a file for project rules,
   the Claude Code skill, the hook, and the Copilot review instructions. If
   your `CLAUDE.md` does not tell about specval, `init` prints a line to add
   to it.

2. Add your services and their layers to `.specval/data/services.yaml`:

   ```yaml
   services:
     OrderManager: { layer: manager }
     PricingEngine: { layer: engine }
     OrderAccessor: { layer: accessor }
   ```

3. Make a spec for a ticket, then write its content:

   ```
   sv new ORD-150 --title "Customer can cancel an order"
   ```

4. Check the spec:

   ```
   sv check
   ```

5. When you implement the change, add its new services to the registry.
   In the same change, set `status: done` in the spec. specval then checks
   only the format of the spec.

6. Make sure that the setup works:

   ```
   sv doctor
   ```

For a complete project, see [`examples/refund`](examples/refund). It has one
spec that passes, one spec that breaks the layer rules, and one spec that
breaks the schema.

## Commands

| Command | What it does |
|---|---|
| `sv check [files...]` | Checks the given specs, or each spec in the specs directory. Use `--format json` for JSON output, and `--strict` to fail on warnings. |
| `sv new <id>` | Makes `<specs dir>/<id>.yaml` from the template. Use `--kind` and `--title` to set those fields. |
| `sv context` | Prints the spec format, the layers, the registry, and the rules. An agent reads this text before it writes a spec. |
| `sv schema` | Prints the JSON Schema of a spec. |
| `sv init [--dry-run]` | Sets up specval, the skill, the hook, and the Copilot review instructions. You can run it again to restore a missing part. With `--dry-run`, it prints the changes and writes nothing. |
| `sv doctor` | Checks the setup, and gives a fix for each failure and warning. Use `--format json` for JSON output. |
| `sv version` | Prints the version of sv and of the spec format. `sv --version` does the same. |
| `sv hook` | Claude Code runs this command after each file write. Do not run it yourself. |

Each evaluation of the rules has a time limit of 10 seconds. If a rule does
not finish in that time, the command fails with exit status 2.

| Exit status | Meaning |
|---|---|
| 0 | The command succeeded and found no errors. |
| 1 | `check` found an error (or a warning, with `--strict`), or `doctor` found a failure. |
| 2 | The command was not correct, or specval could not do it. |

## Documentation

- [docs/specs.md](docs/specs.md): the spec format and the built-in rules.
- [docs/rules.md](docs/rules.md): how to add project data and project rules.
- [docs/claude-code.md](docs/claude-code.md): the skill, the hook, the Copilot review, and `sv doctor`.
- [docs/architecture.md](docs/architecture.md): how specval works inside.
- [CONTRIBUTING.md](CONTRIBUTING.md): how to build, test, and change specval.
