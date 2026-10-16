---
name: spec
description: Write or change the spec for a feature, bugfix, or chore before you write code. Use this skill when you plan work from a ticket, or when you must implement a change that has no passing spec.
---

Each change needs a spec that passes `sv check` before the implementation starts.

1. Run `sv context`. It prints the spec format, the layer rules, the registered services, and each rule.
2. Write `specs/<ticket-id>.yaml`. To make the file from the template, run `sv new <ticket-id> --title "..."`.
3. When you save the file, specval checks it. Each finding has a `fix:`. Apply the fix. Then save the file again. Do this until the spec passes. Do not remove a call or an acceptance criterion that the change needs, only to make a rule pass.
4. Start the implementation only after the spec passes. Implement the calls that the spec lists.
5. When the implementation is complete, add each service in `services.new` to the service registry. In the same change, set `status: done` in the spec. specval then checks only the format of the spec, because the registry now contains its changes.
