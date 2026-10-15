# Project rules

specval has built-in rules (see [specs.md](specs.md)). You can add data and
rules for your project.

## Where the rules come from

| Source | Content | Rego name | Owner |
|---|---|---|---|
| Built into specval | The spec schema, the layer rules, and the SV and SR rules | `data.dpl` | specval |
| `.specval/data/*.yaml`, `*.json` | The service registry and other project data | `data.project.<key>` | The project |
| `.specval/policy/*.rego` | The project rules | `specval.spec.<name>`, `specval.registry.<name>` | The project |

`.specval.yaml` gives the data and policy paths. Each path can be a file or a
directory. specval does not load a Rego file whose name ends with `_test.rego`.

We recommend that you protect `.specval/` with CODEOWNERS. The registry and the
rules control which designs pass.

## Project data

Each top-level key of a data file becomes `data.project.<key>`. For example,
`services` in `.specval/data/services.yaml` is `data.project.services`. Two
data files cannot define the same top-level key.

`data.project.services` is the service registry. It maps each service name to
its layer:

```yaml
services:
  OrderManager: { layer: manager }
  PricingEngine: { layer: engine }
  OrderAccessor: { layer: accessor }
```

## Write a spec rule

A spec rule is in a package `specval.spec.<name>`. specval runs it one time for
each spec, with the spec as `input`.

```rego
package specval.spec.project

import data.specval.lib
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

- Use `deny` for an error and `warn` for a warning. specval reads only these
  two rule names.
- Give each `deny` and `warn` rule a `METADATA` block with a `title` and a
  `custom.id`. specval does not load a rule that has no `custom.id`.
- Make each finding with `lib.finding(rego.metadata.rule(), path, msg, fix)`.
  It takes the ID and the title from the `METADATA` block. Thus
  `specval context` and the findings show the same text.
- Write `path` as a path in the spec, for example `calls[2].to`. specval finds
  the line of that path in the YAML file. If the path is not in the file,
  specval uses the line of the deepest part that is.
- Write `fix` as an instruction. An agent applies the fix without other help.
- Bind each value that can be undefined before you use it in string
  interpolation. `$"{x}"` writes an undefined `x` as `<undefined>`.

The schema rule runs first. If a spec does not match the schema, specval does
not report the findings of your rules. Thus a rule can expect the fields that
the schema requires.

## Write a registry rule

A registry rule is in a package `specval.registry.<name>`. specval runs it one
time for each `check`, with no `input`. Use it to check the project data.

```rego
package specval.registry.project

import data.specval.lib
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
`services.OrderManager`. specval then shows the finding on the data file that
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

`specval context` lists each spec rule, with its ID, severity, title, and
description. `specval doctor` tells you whether your rules compile.
