# Specs

A spec describes one change before anyone writes code for it. It is a YAML
file in the specs directory (`specs/` if you do not change `specs_dir`). Name
the file with the ID of the ticket: `specs/ORD-142.yaml`. specval does not
check the name of the file.

`specval new <id>` makes a spec from the template. `specval schema` prints the
JSON Schema that each spec must match.

## Example

This spec is `examples/refund/specs/ORD-142.yaml`. It passes `specval check`.

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
| `title` | yes | The title of the change, on one line. |
| `summary` | yes | A short paragraph that tells the problem and the new behavior. |
| `acceptance` | yes | One or more acceptance criteria. Each has an `id` (`AC1`, `AC2`, ...) and a `text` that you can test. |
| `services.new` | no | The services that the change adds. Each has a `name`, a `layer`, and a `purpose`. |
| `services.changed` | no | The registered services whose behavior changes. |
| `calls` | yes | Each call from one service to another that the change adds or changes. Each call has `from`, `to`, `op` (the operation, in PascalCase), and `covers` (the acceptance criteria that the call helps to satisfy). |
| `out_of_scope` | no | The things that the change does not do, but that a reader could expect it to do. |

## Layers

specval checks each call against four layers. A service can call only the
layers in its row.

| Layer | Can call | Name ends with |
|---|---|---|
| manager | engine, accessor | `Manager` |
| engine | accessor | `Engine` |
| accessor | resource | `Accessor` |
| resource | (no layer) | `Resource` |

The service registry, `.specval/data/services.yaml`, gives the layer of each
service that exists. A spec gives the layer of each new service in
`services.new`.

## Built-in rules

specval runs the schema check first. If the spec does not match the schema,
specval reports only the schema errors, because the other rules need the
correct shape.

| ID | Severity | Rule |
|---|---|---|
| SV000 | error | The spec is valid YAML. |
| SV001 | error | The spec matches the schema. |
| SV101 | error | Each call uses a registered service or a service in `services.new`. |
| SV102 | error | Each call follows the layer rules. |
| SV103 | error | The name of each new service ends with the name of its layer. |
| SV104 | error | A service in `services.new` is not already registered. |
| SV105 | error | Each new service has a known layer. |
| SV106 | error | Each new service is in one or more calls. |
| SV107 | error | One or more calls cover each acceptance criterion. |
| SV108 | error | Each ID in `covers` is an acceptance criterion of the spec. |
| SV109 | error | Each service in `services.changed` is registered. |
| SV201 | warning | Each service in `services.changed` is in one or more calls. |

specval also runs these rules on the service registry, one time for each
`check`:

| ID | Severity | Rule |
|---|---|---|
| SR001 | error | Each registered service has a known layer. |
| SR002 | error | The name of each registered service ends with the name of its layer. |
| SR003 | error | The registry is a mapping from service names to entries. |
| SR004 | error | Each entry in the registry is a mapping with a layer, for example `{ layer: manager }`. |

`specval context` prints the spec rules with their full descriptions. To add
rules for your project, see [rules.md](rules.md).

## Output

Each finding gives the file, the line, the severity, the rule ID, a message,
and the path in the spec. The line after it gives the fix:

```
specs/ORD-143.yaml:14 error SV102 OrderAccessor (accessor) cannot call PricingEngine (engine) (calls[2])
    fix: Accessors can call only resources. Send the call to PricingEngine through a manager.
```

`specval check --format json` gives the same findings as JSON:

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
