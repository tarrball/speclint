# Roadmap

speclint theorizes that a better spec gives a better result from an AI coding
assistant, and that a linted spec is a better spec. This file holds the work
that tests that idea: the next features, the next experiments, and the ideas
that are not designed yet. The work that is done, and the results of the
first experiments, are in [docs/history.md](docs/history.md).

## Direction

- speclint is for the DPL layers now. It must also work for a brownfield
  project, and later for a strangler migration.
- We test speclint with agents on real projects. We record each result, also
  a bad result, and we change speclint because of it. One run is one sample,
  so a small difference can be noise.
- speclint does not make an architecture. If the registry is wrong, speclint
  makes the code follow the wrong design. A reviewed way to change the
  architecture (`services.relayered`) is part of the product.

## Next: brownfield features

Design these from real brownfield projects on Supabase, with real tickets.
Their layers (for example edge functions and database functions) and their
old code show what these features must do.

- [ ] A project file that changes the layers. It has the shape of `dpl.yaml`,
      and the rules see it as `data.dpl`.
- [ ] Turn a rule off, or change its severity, in `.speclint.yaml`.
- [ ] A `legacy` layer for a strangler migration:
  - Only an accessor can call a legacy service.
  - A legacy service can call only a manager.
  - A spec cannot add a legacy service.
  - speclint warns about a new call between two legacy services.
- [ ] Schema extensions. Then a rule that finds a change to a contract.
- [ ] An `app` field in a registry entry, for a monorepo with more than one
  app, for example `SomeAccessor: { layer: accessor, app: cli }`. In the
  first brownfield trial, the repository had the apps `mcp`, `web`, and `cli`,
  and Claude added the app name as a prefix to each service name
  (`CLISomeAccessor`). That works, but each agent must find its own
  convention, and nothing checks it. With the field:
  - A service without `app` is shared, for example the database.
  - Names stay unique in one registry, so a spec can show a call from one app
    to a shared service.
  - A rule can refuse a call from one app to a service of another app.
  - `slint context` can show the services of each app.
  - Two services of different apps that do the same work are easier to see.

Decision (2026-10-18): a database trigger that stays in one database is
internal behavior of that resource, not a call between two resources. A spec
that adds or changes such a trigger puts the resource in `services.changed`,
with `calls: []`. Thus the resource -> resource rule does not need an override
for triggers. A trigger that goes out of the database, for example to an edge
function or a webhook, is different. The spec format cannot show it yet. That
is an input for the design of the features above.

## Next: experiments

The experiments in the speclint repository had two problems. The agents were
not blind, because the task was about speclint and the code told about specs
and layers. Also, speclint is a CLI, and the DPL layers are for services.
Copilot is not a controlled measure either: it reads the whole repository, and
its result can change from one run to the next.

- [ ] Brownfield trials on Supabase projects: set up speclint, then measure
      tickets with and without speclint.
- [ ] A separate repository with a small DPL service: HTTP endpoints as
      clients, managers, engines, accessors, and a database as a resource.
      Later, add an old version with layer violations.
- [ ] Measure with our own checks: the tests pass, the imports agree with the
      registry after the change, and the code has each call of the spec and no
      other calls. Also record the time, tokens, tool calls, and diff size.
- [ ] A reviewer agent with a fixed list of checks. It sees only the diff and
      the target architecture. Copilot is a second opinion only.
- [ ] Run each arm 3 or more times.

## Ideas

These are not designed yet. Each one needs a decision before it becomes work.

- **A Copilot flag for `slint init`.** `slint init --claude` adds the Claude
  Code parts only when you ask for them. The Copilot review instructions are
  still on by default. A `--copilot` flag would make each integration opt-in
  in the same way (a field in `scaffold.Options` and a flag on `init`).
- **Security boundaries.** A call that crosses a security boundary may need
  more than the layer rules. Examples: a client that calls a service that
  uses elevated privileges, or an edge function that uses the Supabase
  service role to go around row level security. Such a call can trigger a
  warning, a required acceptance criterion, or a review. Questions: how does
  the registry mark a boundary (for example a `zone` or `trust` field on a
  service), and what does a crossing trigger? See also the authorization
  idea below.
- **Generated Mermaid diagrams.** slint can write a Mermaid diagram of the
  registry (the services by layer, with their calls), or of a spec (the
  calls of the change, with the new, changed, and removed calls marked). A
  pull request description or the Copilot review can show the diagram. It
  could be a new command or a `--format mermaid` option.
- **Authorization in a spec.** The first brownfield trial (2026-10-18) showed
  that a spec gives the calls of a change, but not who can do the change. An
  acceptance criterion such as "an admin can do this" or "the manager of the
  team can do this" is only text. Nothing connects it to a call, a layer, or
  a check in the code. Questions to answer before a design:
  - The shape. One option: each acceptance criterion or call names an actor
    and a scope, for example `{ actor: team-manager, scope: team }`. The
    actors come from project data (`data.project.roles`), so a spec can use
    only known actors.
  - The layer that does the check. In DPL, the manager can check access, or a
    security utility can do it. In a Supabase project, row level security
    does the check in the resource. A rule can then make sure that each
    change that has an actor goes through the service that checks access.
  - Baseline or project. Each project has different roles. Maybe this is a
    schema extension, not a built-in field.
  - Review. The Copilot instructions can compare the access checks in the
    code with the actors of the spec.
- **A client that calls more than one manager.** Warn about it. It can show a
  problem in the decomposition, because a manager does the orchestration.
- **The file name of a spec is its `id`.** Today `slint new` writes
  `<id>.yaml`, but nothing finds a file name and an `id` that do not agree.
  The rules see only the content of the spec. With this rule, a project rule
  on `input.id` also controls the file names, so speclint needs no other
  feature for a naming convention. The convention is the choice of each
  project, for example a ticket key (`PROJ-123-short-name`) or a date.
- **A comma in a flow mapping.** `{ purpose: A, B, and C }` makes YAML read
  `B` and `and C` as keys, and SV001 says "Remove `and`". If a key has a
  space, the fix can tell you to quote the value.
- **Numbered specs for speclint itself.** A number prefix
  (`0003-extract-check-manager`), because the specs build on each other, and
  the number gives their order.
