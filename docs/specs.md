# Specs

A spec describes one change before anyone writes code for it. It is a YAML
file in the specs directory (`specs/` if you do not change `specs_dir`). Name
the file with the ID of the ticket: `specs/ORD-142.yaml`. specval does not
check the name of the file.

`sv new <id>` makes a spec from the template. `sv schema` prints the
JSON Schema that each spec must match.

## Example

This spec is `examples/refund/specs/ORD-142.yaml`. It passes `sv check`.

```yaml
specval: 1
id: ORD-142
kind: feature
title: Customer can request a refund for a delivered order
summary: >
  Customers must call support to get a refund. Let them request a refund for
  a delivered order. The refund amount follows the pricing rules that applied
  at the time of purchase.
acceptance:
  - id: AC1
    text: Given a delivered order, when the customer requests a refund, then the system calculates the refund amount from the original price and discounts.
  - id: AC2
    text: Given an order that is not delivered, when the customer requests a refund, then the system rejects the request and gives the reason.
  - id: AC3
    text: Given an approved refund, then the order records the refund amount and status.
services:
  new:
    - { name: RefundEngine, layer: engine, purpose: Calculates the refund eligibility and amount for an order }
  changed: [OrderManager, OrderAccessor]
calls:
  - { from: OrderManager, to: OrderAccessor, op: GetOrder, covers: [AC1, AC2] }
  - { from: OrderManager, to: PricingEngine, op: GetAppliedPricing, covers: [AC1] }
  - { from: OrderManager, to: RefundEngine, op: CalculateRefund, covers: [AC1, AC2] }
  - { from: OrderManager, to: OrderAccessor, op: RecordRefund, covers: [AC3] }
out_of_scope:
  - Partial refunds of individual line items
```

## Fields

| Field | Required | Content |
|---|---|---|
| `specval` | yes | The version of the spec format. It is always `1`. |
| `id` | yes | The ID of the ticket, for example `ORD-142`. Use letters, digits, `.`, `_`, and `-`. |
| `kind` | yes | `feature`, `bugfix`, or `chore`. |
| `status` | no | `draft` or `done`. A spec without a status is a draft. See [The life of a spec](#the-life-of-a-spec). |
| `title` | yes | The title of the change, on one line. |
| `summary` | yes | A short paragraph that tells the problem and the new behavior. |
| `acceptance` | yes | One or more acceptance criteria. Each has an `id` (`AC1`, `AC2`, ...) and a `text` that you can test. |
| `services.new` | no | The services that the change adds. Each has a `name`, a `layer`, and a `purpose`. |
| `services.changed` | no | The registered services whose behavior changes. |
| `services.relayered` | no | The registered services that the change moves to another layer. Each has a `name` (the name today), a `layer` (the new layer), a `reason`, and an optional `rename`. See [Move a service to another layer](#move-a-service-to-another-layer). |
| `calls` | yes | Each call from one service to another that the change adds or changes. Each call has `from`, `to`, `op` (the operation, in PascalCase), and `covers` (the acceptance criteria that the call helps to satisfy). A change inside one service has `calls: []`. |
| `removed_calls` | no | Each call that the change removes. Each item has `from` and `to`. See [Calls today](#calls-today). |
| `out_of_scope` | no | The things that the change does not do, but that a reader could expect it to do. |

## The life of a spec

A spec describes one change at one time. specval checks it against the
service registry as the registry is now. Thus a spec has two states:

1. `draft`: you plan the change, and then you implement it. specval checks
   the spec with all of the rules.
2. `done`: the change is implemented. specval checks only the format of the
   spec (SV000 and SV001).

When you implement a spec, do these steps in the same change:

1. Add each service in `services.new` to the service registry.
2. For each service in `services.relayered`, change its layer in the
   registry. If it has a `rename`, change its name in the registry, and in
   each `calls`, `may_call`, and `called_by` list that has the name.
3. Update the `calls` of each service in the registry: add the calls of the
   spec, and remove the calls in `removed_calls`.
4. Set `status: done` in the spec.

If the spec stays a draft, its new services are now in the registry, so
SV104 reports each one.

## Layers

specval checks each call against six layers. A service can call only the
layers in its row.

| Layer | Can call | Suffix of the service name |
|---|---|---|
| client | manager, utility | (none) |
| manager | engine, accessor, utility | `Manager` |
| engine | accessor, utility | `Engine` |
| accessor | resource, utility | `Accessor` |
| resource | utility | `Resource` |
| utility | utility | (none) |

- A client is an entry point, for example an API endpoint or a function.
  It calls a manager, and the manager does the orchestration.
- A utility is a service that each layer can use, for example logging.

### Limits for one service

A registry entry can make the layer rules stricter for one service:

- `may_call` lists the services that the service can call.
- `called_by` lists the services that can call the service.

Each value is a layer or a registered service. The name of a layer always
means the layer. An empty list permits no service. For example, other utilities can call `ConfigUtility`, but it cannot
call a utility:

```yaml
services:
  ConfigUtility: { layer: utility, may_call: [] }
  AuditUtility: { layer: utility, called_by: [manager] }
```

A limit cannot permit a call that the layer rules do not permit. SV111
reports a call that a limit refuses. A new service in `services.new` has no
limits.

### A change inside one service

Some changes add no call between services, for example a new help text in a
command. Such a spec has `calls: []`, and it puts each service that it changes
in `services.changed`:

```yaml
services:
  changed: [CheckCommand, DoctorCommand]
calls: []
```

The services in `services.changed` then cover each acceptance criterion
(SV107), and SV201 does not apply. If the spec has no calls and no service in
`services.changed`, SV118 reports it.

### Calls today

`calls` in a registry entry lists the services that the service calls today:

```yaml
services:
  HookCommand: { layer: client, calls: [ConfigAccessor] }
```

The registry records each call, also a call that breaks the layer rules. In an
old project, such calls are normal. specval does not report them until a spec
changes the service:

- For each service in `services.changed`, SV202 checks each call that the
  service keeps after the change. If a kept call breaks the layer rules or a
  limit, SV202 gives a warning.
- To fix the warning, put the call in `removed_calls`, and add the calls that
  do the work in a correct way. For example, the client calls a manager, and
  the manager calls the accessor.

Thus each change makes the services that it changes follow the layers.

### Move a service to another layer

specval does not make the architecture. If the registry puts a service in the
wrong layer, the rules make the code follow the wrong design. For example, a
format that each layer uses is a utility, not an accessor.

To move a registered service, put it in `services.relayered`:

```yaml
services:
  relayered:
    - name: ClaudeAccessor
      layer: utility
      rename: ClaudeHook
      reason: The hook event is a format, not stored data.
calls:
  - { from: HookCommand, to: ClaudeHook, op: ReadEvent, covers: [AC1] }
removed_calls:
  - { from: ClaudeHook, to: StoreResource }
```

- `name` is the name of the service today. The rest of the spec uses the new
  name.
- If the name does not end with the suffix of the new layer, `rename` gives
  the new name (SV114). A client or a utility has no suffix, so `rename` is
  optional for them.
- The rules check the spec with the registry after the move. SV203 checks each
  registered call to and from the moved service with the new layer. Put each
  call that breaks the layers in `removed_calls`, with the new name.
- A reviewer of the spec sees the move and its `reason`. A move changes the
  architecture, so review it as such.

The service registry, `.specval/data/services.yaml`, gives the layer of each
service that exists. A spec gives the layer of each new service in
`services.new`. The `package` of a registry entry tells where the code of the
service is (see [Project data](rules.md#project-data)).

## Built-in rules

specval runs the schema check first. If the spec does not match the schema,
specval reports only the schema errors, because the other rules need the
correct shape. The text output then gives a note for the spec:

```
specs/ORD-7.yaml note: The other rules run after you fix the format errors (SV000, SV001).
```

Thus when you fix the format errors, the next check can find new problems.

| ID | Severity | Rule |
|---|---|---|
| SV000 | error | The spec is valid YAML, with one document. |
| SV001 | error | The spec matches the schema. |
| SV101 | error | Each call uses a registered service or a service in `services.new`. |
| SV102 | error | Each call follows the layer rules. |
| SV103 | error | The name of each new service ends with the suffix of its layer. |
| SV104 | error | A service in `services.new` is not already registered. |
| SV105 | error | Each new service has a known layer. |
| SV106 | error | Each new service is in one or more calls. |
| SV107 | error | One or more calls cover each acceptance criterion. In a spec with no calls, the services in `services.changed` cover them. |
| SV108 | error | Each ID in `covers` is an acceptance criterion of the spec. |
| SV109 | error | Each service in `services.changed` is registered. |
| SV110 | error | Each name in `services.new` is there one time only. |
| SV111 | error | Each call follows the limits in the registry (`may_call` and `called_by`). |
| SV112 | error | Each call in `removed_calls` is in the `calls` of its service in the registry. |
| SV113 | error | Each service in `services.relayered` is registered, with its name today. |
| SV114 | error | The name of a moved service ends with the suffix of its new layer. If it does not, `rename` gives the new name. |
| SV115 | error | Each moved service has a known layer. |
| SV116 | error | Each name in `services.relayered` is there one time only. |
| SV117 | error | The `rename` of a moved service is not the name of another service, a layer, or another new name. |
| SV118 | error | A spec with no calls has one or more services in `services.changed`. |
| SV201 | warning | In a spec with calls, each service in `services.changed` is in one or more calls. |
| SV202 | warning | Each call that a changed service keeps follows the layer rules and the limits. |
| SV203 | warning | Each registered call to or from a moved service that the spec keeps follows the layer rules and the limits after the move. |

specval also runs these rules on the service registry, one time for each
`check`:

| ID | Severity | Rule |
|---|---|---|
| SR001 | error | Each registered service has a known layer. |
| SR002 | error | The name of each registered service ends with the suffix of its layer. |
| SR003 | error | The registry is a mapping from service names to entries. |
| SR004 | error | Each entry in the registry is a mapping with a layer, for example `{ layer: manager }`. |
| SR005 | error | Each limit (`may_call` or `called_by`) is a list of layers and registered services. |
| SR006 | warning | A limit does not permit a call that the layer rules do not permit. |
| SR007 | error | The name of a registered service is not the name of a layer. |
| SR008 | error | `calls` in a registry entry is a list of registered services. |

`sv context` prints the spec rules with their full descriptions. To add
rules for your project, see [rules.md](rules.md).

## Output

Each finding gives the file, the line, the severity, the rule ID, a message,
and the path in the spec. The line after it gives the fix:

```
specs/ORD-143.yaml:14 error SV102 OrderAccessor (accessor) cannot call PricingEngine (engine) (calls[2])
    fix: Accessors can call only resources. Send the call to PricingEngine through a manager.
```

`sv check --format json` gives the same findings as JSON:

```json
{
  "files": [
    {
      "file": "specs/ORD-143.yaml",
      "findings": [
        {
          "rule": "SV102",
          "title": "Calls follow the layer rules",
          "severity": "error",
          "path": "calls[2]",
          "line": 14,
          "msg": "OrderAccessor (accessor) cannot call PricingEngine (engine)",
          "fix": "Accessors can call only resources. Send the call to PricingEngine through a manager."
        }
      ]
    }
  ],
  "errors": 1,
  "warnings": 0
}
```

A warning does not fail `check`, unless you use `--strict`.
