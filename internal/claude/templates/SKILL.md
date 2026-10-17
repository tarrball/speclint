---
name: spec
description: Write or change the spec for a feature, bugfix, or chore before you write code. Use this skill when you plan work from a ticket, or when you must implement a change that has no passing spec.
---

Each change needs a spec that passes `sv check` before the implementation starts.

1. Run `sv context`. It prints the spec format, the layer rules, the registered services, and each rule.
2. Write `{{SPECS_DIR}}/<ticket-id>.yaml`. To make the file from the template, run `sv new <ticket-id> --title "..."`. If the change adds no call between services, for example a change inside one service, write `calls: []` and put each changed service in `services.changed`.
3. When you save the file with Write or Edit, a hook checks it. If you write the file in another way, for example with a shell command, run `sv check <file>`. Each finding has a `fix:`. Apply the fix, and check the spec again. Do this until the spec passes. Do not remove a call or an acceptance criterion that the change needs, only to make a rule pass.
4. Start the implementation only after the spec passes. Implement the calls that the spec lists.
5. When the implementation is complete, update the service registry: add each service in `services.new`, move each service in `services.relayered`, and update the `calls` of each service. In the same change, set `status: done` in the spec. Then run `sv check`. It checks each spec and the registry. For a spec with `status: done`, it checks only the format, because the registry now contains its changes.

`sv check --format json` gives the findings as JSON. The exit status of `sv check` is 0 if the specs pass, 1 if there is an error, and 2 if specval cannot check them. For the flags and examples of a command, run `sv <command> --help`.
