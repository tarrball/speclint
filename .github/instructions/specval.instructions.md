---
applyTo: "**"
excludeAgent: "coding-agent"
---

# Compare the pull request with its spec

This project uses specval. Each change has a spec in `specs/<id>.yaml`.
The spec gives the services that the change adds, changes, or moves, the calls
between the services, and the acceptance criteria. The registry,
`.specval/data/services.yaml`, gives the layer and the calls of each service today. The
`package` of a service, if the registry gives one, tells where its code is.

Do your usual review first. Then, at the end of your review summary, add a
section with the title "Spec deviations". This section shows how well the
author or the coding agent followed the spec. It is for information only. Do
not request changes only because of a deviation.

1. Find the spec of the pull request: a spec that the pull request adds or
   changes, or the spec whose `id` the title, the branch name, or the
   description names.
   - If the pull request changes code and there is no spec, write "No spec".
     Then stop.
   - If the pull request changes only specs, write "Spec only". Then stop.
2. Compare the code with the spec. Report each deviation on one line, with the
   field of the spec and the file of the code:
   - A call in `calls` that the code does not make.
   - A call between two services in the code that is not in `calls` and not
     in the registry.
   - A call in `removed_calls` that the code still makes.
   - A service in `services.new` that the code does not add, or that has a
     different name or layer.
   - A service in `services.relayered` that the code does not move or rename.
   - A change in a service that is not in the spec.
   - An acceptance criterion that no test checks.
   - A change to an item in `out_of_scope`.
3. When the code implements the spec, the registry must agree with it in the
   same pull request: each new service, moved service, new call, and removed
   call. The spec must have `status: done`. Report each difference.
4. If you find no deviation, write "None".
