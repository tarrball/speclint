package specval.spec.dpl

import data.specval.lib
import rego.v1

registry := object.get(data.project, "services", {})

layers := data.dpl.layers

suffix := data.dpl.suffix

calls := object.get(input, "calls", [])

acceptance := object.get(input, "acceptance", [])

new_list := object.get(input, ["services", "new"], [])

changed := object.get(input, ["services", "changed"], [])

new_services[s.name] := s if {
	some s in new_list
}

ac_ids contains ac.id if {
	some ac in acceptance
}

used contains name if {
	some c in calls
	some name in [c.from, c.to]
}

covered contains id if {
	some c in calls
	some id in object.get(c, "covers", [])
}

layer_of(name) := registry[name].layer

layer_of(name) := new_services[name].layer if not registry[name]

# suggestions returns the registered services in the layer that the suffix of
# name gives. If there are none, it returns all of the registered services.
suggestions(name) := names if {
	some l, sfx in suffix
	endswith(name, sfx)
	names := sort([n | some n, s in registry; s.layer == l])
	count(names) > 0
} else := sort(object.keys(registry))

base_name(name) := trim_suffix(name, sfx) if {
	some sfx in suffix
	endswith(name, sfx)
} else := name

can_call(l) := $"{lib.capitalize(lib.plural(l))} may call only {targets}" if {
	count(layers[l]) > 0
	targets := lib.join([lib.plural(t) | some t in lib.ordered(layers[l])], "and")
}

can_call(l) := $"{lib.capitalize(lib.plural(l))} may not call other services" if count(layers[l]) == 0

# layer_fix is undefined if the layer of the caller is not known (SV105).
# String interpolation writes an undefined value as "<undefined>", so the
# rules bind each value that can be undefined before they use it.
layer_fix(from, to, target) := msg if {
	rule := can_call(from)
	callers := lib.ordered([l | some l, targets in layers; to in targets])
	count(callers) > 0
	msg := $"{rule}. Route the call to {target} through a {lib.join(callers, "or")} instead."
} else := msg if {
	rule := can_call(from)
	callee := lib.capitalize(lib.plural(to))
	msg := $"{rule}. {callee} are entry points; no service calls them."
}

# unknown_fix tells how to fix a reference to a service that is not registered.
# It is undefined if the registry is not an object. SR003 reports that case.
unknown_fix(name, action) := concat(" ", [
	$"If {name} is new, {action} services.new with a layer and purpose.",
	$"Otherwise use a registered service: {names}.",
]) if {
	names := concat(", ", suggestions(name))
}

# METADATA
# title: Calls use known services
# description: Each call's `from` and `to` is a registered service or is declared in `services.new`.
# custom:
#   id: SV101
deny contains f if {
	some i, c in calls
	some end in ["from", "to"]
	name := c[end]
	not registry[name]
	not new_services[name]
	f := lib.finding(
		rego.metadata.rule(),
		$"calls[{i}].{end}",
		$"{name} is not a registered service",
		unknown_fix(name, "declare it under"),
	)
}

# METADATA
# title: Calls follow the layer rules
# description: A service may call only the layers its own layer allows (see Layers).
# custom:
#   id: SV102
deny contains f if {
	some i, c in calls
	from := layer_of(c.from)
	to := layer_of(c.to)
	not to in object.get(layers, from, [])
	f := lib.finding(
		rego.metadata.rule(),
		$"calls[{i}]",
		$"{c.from} ({from}) may not call {c.to} ({to})",
		layer_fix(from, to, c.to),
	)
}

# METADATA
# title: New services are named for their layer
# description: A new service's name ends with its layer's suffix (Manager, Engine, Accessor, Resource).
# custom:
#   id: SV103
deny contains f if {
	some i, s in new_list
	sfx := suffix[s.layer]
	not endswith(s.name, sfx)
	f := lib.finding(
		rego.metadata.rule(),
		$"services.new[{i}].name",
		$"{s.name} is in the {s.layer} layer but does not end in {sfx}",
		$"Rename it to {base_name(s.name)}{sfx} everywhere in the spec, or change its layer.",
	)
}

# METADATA
# title: New services are actually new
# description: A service already in the registry belongs under `services.changed`, not `services.new`.
# custom:
#   id: SV104
deny contains f if {
	some i, s in new_list
	layer := registry[s.name].layer
	f := lib.finding(
		rego.metadata.rule(),
		$"services.new[{i}].name",
		$"{s.name} is already registered ({layer} layer)",
		$"Move {s.name} from services.new to services.changed.",
	)
}

# METADATA
# title: New services use a known layer
# description: A new service's `layer` is one of the DPL layers.
# custom:
#   id: SV105
deny contains f if {
	some i, s in new_list
	not layers[s.layer]
	f := lib.finding(
		rego.metadata.rule(),
		$"services.new[{i}].layer",
		$"{s.layer} is not a layer",
		$"Use one of: {concat(", ", data.dpl.layer_order)}.",
	)
}

# METADATA
# title: New services are used
# description: Every service in `services.new` appears in at least one call.
# custom:
#   id: SV106
deny contains f if {
	some i, s in new_list
	not used[s.name]
	f := lib.finding(
		rego.metadata.rule(),
		$"services.new[{i}]",
		$"{s.name} is declared as new but no call uses it",
		$"Add the calls to or from {s.name}, or remove it from services.new.",
	)
}

# METADATA
# title: Every acceptance criterion is implemented by a call
# description: >-
#   Each acceptance criterion id appears in the `covers` list of at least one call,
#   so every requirement traces to the services that implement it.
# custom:
#   id: SV107
deny contains f if {
	some i, ac in acceptance
	not covered[ac.id]
	f := lib.finding(
		rego.metadata.rule(),
		$"acceptance[{i}]",
		$"{ac.id} is not covered by any call",
		$"Add {ac.id} to `covers` on the call(s) that implement it, or add the missing call.",
	)
}

# METADATA
# title: Calls cover real acceptance criteria
# description: Every id in a call's `covers` is defined under `acceptance`.
# custom:
#   id: SV108
deny contains f if {
	some i, c in calls
	some j, id in object.get(c, "covers", [])
	not ac_ids[id]
	f := lib.finding(
		rego.metadata.rule(),
		$"calls[{i}].covers[{j}]",
		$"{id} is not an acceptance criterion in this spec",
		$"Use one of: {concat(", ", sort(ac_ids))}, or add {id} under acceptance.",
	)
}

# METADATA
# title: Changed services are registered
# description: Every name in `services.changed` is in the registry.
# custom:
#   id: SV109
deny contains f if {
	some i, name in changed
	not registry[name]
	f := lib.finding(
		rego.metadata.rule(),
		$"services.changed[{i}]",
		$"{name} is not a registered service",
		unknown_fix(name, "move it to"),
	)
}

# METADATA
# title: Changed services take part in a call
# description: A service listed in `services.changed` should appear in at least one call.
# custom:
#   id: SV201
warn contains f if {
	some i, name in changed
	registry[name]
	not used[name]
	f := lib.finding(
		rego.metadata.rule(),
		$"services.changed[{i}]",
		$"{name} is listed as changed but no call involves it",
		$"Add the calls that change {name}, or remove it from services.changed.",
	)
}
