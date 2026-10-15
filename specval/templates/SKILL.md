---
name: spec
description: Write or revise the spec (specs/<ticket-id>.yaml) for a feature, bugfix, or chore before any code is written. Use when planning work from a ticket or when asked to implement something that has no passing spec yet.
---

Every change needs a spec that passes `specval check` before implementation starts.

1. Run `specval context`. It prints the spec format, the layer rules, the registered services, and every rule.
2. Write `specs/<ticket-id>.yaml` (`specval new <ticket-id> --title "..."` scaffolds it).
3. Saving the file runs the checks automatically. Each finding has a `fix:`; apply it and save again until the spec passes. Do not satisfy a rule by dropping calls or acceptance criteria the change really needs.
4. Start implementation only once the spec passes, and implement the calls it lists.
