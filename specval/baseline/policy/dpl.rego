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

new_services := {s.name: s | some s in new_list}

ac_ids := {ac.id | some ac in acceptance}

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

# Registered services in the same layer as `name` (by suffix), else all of them.
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

can_call(l) := sprintf("%s may call only %s", [
	lib.capitalize(lib.plural(l)),
	lib.join([lib.plural(t) | some t in lib.ordered(layers[l])], "and"),
]) if count(layers[l]) > 0

can_call(l) := sprintf("%s may not call other services", [lib.capitalize(lib.plural(l))]) if count(layers[l]) == 0

layer_fix(from, to, target) := msg if {
	callers := lib.ordered([l | some l, targets in layers; to in targets])
	count(callers) > 0
	msg := sprintf("%s. Route the call to %s through a %s instead.", [can_call(from), target, lib.join(callers, "or")])
} else := sprintf("%s. %s are entry points; no service calls them.", [can_call(from), lib.capitalize(lib.plural(to))])

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
		sprintf("calls[%d].%s", [i, end]),
		sprintf("%s is not a registered service", [name]),
		sprintf("If %s is new, declare it under services.new with a layer and purpose. Otherwise use a registered service: %s.", [name, concat(", ", suggestions(name))]),
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
		sprintf("calls[%d]", [i]),
		sprintf("%s (%s) may not call %s (%s)", [c.from, from, c.to, to]),
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
		sprintf("services.new[%d].name", [i]),
		sprintf("%s is in the %s layer but does not end in %s", [s.name, s.layer, sfx]),
		sprintf("Rename it to %s%s everywhere in the spec, or change its layer.", [base_name(s.name), sfx]),
	)
}

# METADATA
# title: New services are actually new
# description: A service already in the registry belongs under `services.changed`, not `services.new`.
# custom:
#   id: SV104
deny contains f if {
	some i, s in new_list
	reg := registry[s.name]
	f := lib.finding(
		rego.metadata.rule(),
		sprintf("services.new[%d].name", [i]),
		sprintf("%s is already registered (%s layer)", [s.name, reg.layer]),
		sprintf("Move %s from services.new to services.changed.", [s.name]),
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
		sprintf("services.new[%d].layer", [i]),
		sprintf("%v is not a layer", [s.layer]),
		sprintf("Use one of: %s.", [concat(", ", data.dpl.layer_order)]),
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
		sprintf("services.new[%d]", [i]),
		sprintf("%s is declared as new but no call uses it", [s.name]),
		sprintf("Add the calls to or from %s, or remove it from services.new.", [s.name]),
	)
}

# METADATA
# title: Every acceptance criterion is implemented by a call
# description: Each acceptance criterion id appears in the `covers` list of at least one call, so every requirement traces to the services that implement it.
# custom:
#   id: SV107
deny contains f if {
	some i, ac in acceptance
	not covered[ac.id]
	f := lib.finding(
		rego.metadata.rule(),
		sprintf("acceptance[%d]", [i]),
		sprintf("%s is not covered by any call", [ac.id]),
		sprintf("Add %s to `covers` on the call(s) that implement it, or add the missing call.", [ac.id]),
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
		sprintf("calls[%d].covers[%d]", [i, j]),
		sprintf("%s is not an acceptance criterion in this spec", [id]),
		sprintf("Use one of: %s, or add %s under acceptance.", [concat(", ", sort(ac_ids)), id]),
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
		sprintf("services.changed[%d]", [i]),
		sprintf("%s is not a registered service", [name]),
		sprintf("If %s is new, move it to services.new with a layer and purpose. Otherwise use a registered service: %s.", [name, concat(", ", suggestions(name))]),
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
		sprintf("services.changed[%d]", [i]),
		sprintf("%s is listed as changed but no call involves it", [name]),
		sprintf("Add the calls that change %s, or remove it from services.changed.", [name]),
	)
}
