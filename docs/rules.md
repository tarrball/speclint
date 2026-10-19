# Project rules

speclint has built-in rules (see [specs.md](specs.md)). You can add data and
rules for your project.

## Where the rules come from

| Source | Content | Rego name | Owner |
|---|---|---|---|
| Built into speclint | The spec schema and the layer data | `data.dpl` | speclint |
| Built into speclint | The SV and SR rules | `speclint.spec.schema`, `speclint.spec.dpl`, `speclint.registry.dpl` | speclint |
| `.speclint/data/*.yaml`, `*.json` | The service registry and other project data | `data.project.<key>` | The project |
| `.speclint/policy/*.rego` | The project rules | `speclint.spec.<name>`, `speclint.registry.<name>` | The project |

`.speclint.yaml` gives the data and policy paths. Each path can be a file or a
directory. speclint does not load a Rego file whose name ends with `_test.rego`.

We recommend that you protect `.speclint/` with CODEOWNERS. The registry and the
rules control which designs pass.

## Project data

Each top-level key of a data file becomes `data.project.<key>`. For example,
`services` in `.speclint/data/services.yaml` is `data.project.services`. Two
data files cannot define the same top-level key.

`data.project.services` is the service registry. It maps each service name to
its layer:

```yaml
services:
  OrderManager: { layer: manager, package: src/orders, calls: [PricingEngine, OrderAccessor] }
  PricingEngine: { layer: engine, package: src/pricing }
  OrderAccessor: { layer: accessor, package: src/orders/db }
```

Each entry must have `layer`. It can also have these fields:

- `package` tells where the code of the service is, for example a directory
  or a file. A person or an agent who writes a spec uses it to find the code.
  `slint context` shows it, and the Copilot review instructions use it to compare
  the code with the spec. speclint checks only that it is text (SR009), not
  that the path exists. Several services can have the same `package`.
- `calls` lists the services that the service calls today. See
  [Calls today](specs.md#calls-today).
- `may_call` and `called_by` make the layer rules stricter for the service.
  See [Limits for one service](specs.md#limits-for-one-service).

## Write a spec rule

A spec rule is in a package `speclint.spec.<name>`. speclint runs it one time for
each spec, with the spec as `input`. The name must be one word: speclint does
not load a package such as `speclint.spec.team.pricing`.

```rego
package speclint.spec.project

import data.speclint.lib
import rego.v1

# METADATA
# title: Only OrderManager calls PricingEngine
# description: One service controls pricing, so that all discounts apply in the same way.
# custom:
#   id: ORD001
deny contains f if {
	some i, c in input.calls
	c.to == "PricingEngine"
	c.from != "OrderManager"
	f := lib.finding(
		rego.metadata.rule(),
		$"calls[{i}]",
		$"{c.from} cannot call PricingEngine",
		"Make OrderManager call PricingEngine, and give the price to the services that need it.",
	)
}
```

Follow these rules:

- Use `deny` for an error and `warn` for a warning. speclint reads only these
  two rule names.
- Give each `deny` and `warn` rule a `METADATA` block with a `title` and a
  `custom.id`. speclint does not load a rule that has no `custom.id`.
- Make each finding with `lib.finding(rego.metadata.rule(), path, msg, fix)`.
  It takes the ID and the title from the `METADATA` block. Thus
  `slint context` and the findings show the same text.
- Write `path` as a path in the spec, for example `calls[2].to`. speclint finds
  the line of that path in the YAML file. If the path is not in the file,
  speclint uses the line of the deepest part that is.
- If a key in the path comes from the data, use `lib.path_key(prefix, key)`.
  It writes a key with a dot, a bracket, or a space in brackets and quotes,
  for example `services["Order.Accessor"]`, so that speclint finds its line.
- Write `fix` as an instruction. An agent applies the fix without other help.
- Bind each value that can be undefined before you use it in string
  interpolation. `$"{x}"` writes an undefined `x` as `<undefined>`.
- Do not use a built-in function that can give a different result each time,
  for example `http.send`, `time.now_ns`, `rand.intn`, or `uuid.rfc4122`.
  speclint does not load a rule that uses one. Thus a spec gets the same
  findings each time, and a rule cannot use the network.
- Do not use `json.match_schema` or `json.verify_schema`. A schema can load a
  remote `$ref`. Only the built-in rule SV001 can use these functions.

The schema rule runs first. If a spec does not match the schema, speclint does
not report the findings of your rules. Thus a rule can expect the fields that
the schema requires.

## Write a registry rule

A registry rule is in a package `speclint.registry.<name>`. speclint runs it one
time for each `check`, with no `input`. Use it to check the project data.

```rego
package speclint.registry.project

import data.speclint.lib
import rego.v1

# METADATA
# title: Each manager has an owner
# custom:
#   id: PRJ900
warn contains f if {
	some name, s in data.project.services
	s.layer == "manager"
	not s.owner
	f := lib.finding(
		rego.metadata.rule(),
		$"services.{name}",
		$"{name} has no owner",
		$"Add owner to {name}.",
	)
}
```

The first part of `path` must be a top-level data key, for example
`services.OrderManager`. speclint then shows the finding on the data file that
defines that key, at the line of that path.

## Rule IDs

The built-in rules use these IDs. Use a different prefix for your project, for
example the prefix of your tickets.

| IDs | Use |
|---|---|
| SV000 | YAML syntax |
| SV001 | The schema |
| SV1xx | Built-in spec errors |
| SV2xx | Built-in spec warnings |
| SR0xx | Built-in registry rules |

## See the rules

`slint context` lists each spec rule, with its ID, severity, title, and
description. `slint doctor` tells you whether your rules compile.
