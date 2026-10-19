package speclint.spec.dpl

import data.speclint.lib
import rego.v1

# current is the registry today. The registry rules (SR001 to SR008) check it.
current := object.get(data.project, "services", {})

# registry is the registry after the moves in services.relayered: each moved
# service has its new layer and its new name. The spec rules use it, because
# the rest of the spec uses the new names. See relayer.rego.
registry := moved_registry if is_object(current)

else := current

layers := data.dpl.layers

suffix := data.dpl.suffix

calls := object.get(input, "calls", [])

acceptance := object.get(input, "acceptance", [])

new_list := object.get(input, ["services", "new"], [])

changed := object.get(input, ["services", "changed"], [])

removed := object.get(input, "removed_calls", [])

# removed_pairs holds each call in removed_calls as [from, to].
removed_pairs contains [r.from, r.to] if {
	some r in removed
}

# registered_calls returns the calls of a service in the registry, or [] if
# the registry has no list of calls for it. SR008 checks the lists.
registered_calls(name) := list if {
	list := registry[name].calls
	is_array(list)
} else := []

# new_services maps each name in services.new to its first entry. If a name
# is in services.new more than one time, SV110 reports it. The other rules
# use the first entry, so that two entries cannot cause a conflict.
new_services[s.name] := s if {
	some i, s in new_list
	not repeated(i, s.name)
}

# repeated is true if an entry before index i of services.new has the name.
repeated(i, name) if {
	some j, t in new_list
	j < i
	t.name == name
}

ac_ids contains ac.id if {
	some ac in acceptance
}

used contains name if {
	some c in calls
	some name in [c.from, c.to]
}

# A change that removes a call also uses the services of the call.
used contains name if {
	some r in removed
	some name in [r.from, r.to]
}

covered contains id if {
	some c in calls
	some id in object.get(c, "covers", [])
}

# has_calls is true if the spec has calls. A spec with no calls is a change
# inside the services in services.changed, and they cover each acceptance
# criterion. SV118 reports a spec that has no calls and no changed service.
has_calls if count(calls) > 0

layer_of(name) := registry[name].layer

layer_of(name) := new_services[name].layer if not registry[name]

# suggestions returns the registered services in the layer that the suffix of
# name gives. If there are none, it returns all of the registered services.
# It is undefined if the registry is not an object.
suggestions(name) := names if {
	some l, sfx in suffix
	endswith(name, sfx)
	names := sort([n | some n, s in registry; s.layer == l])
	count(names) > 0
} else := sort(object.keys(registry)) if is_object(registry)

base_name(name) := trim_suffix(name, sfx) if {
	some sfx in suffix
	endswith(name, sfx)
} else := name

# permits is true if list has the service name or the layer of the service.
# A value that is the name of a layer always means the layer, as in SR006.
# SR007 refuses a service with the name of a layer.
permits(list, name) if {
	name in list
	not layers[name]
}

permits(list, name) if layer_of(name) in list

# follows_layers is true if the layer rules permit the call. SV111 checks
# only these calls, so that SV102 and SV111 do not report the same call.
follows_layers(c) if {
	from := layer_of(c.from)
	to := layer_of(c.to)
	to in object.get(layers, from, [])
}

# refused_by_limits is true if may_call of the caller or called_by of the
# callee refuses the call.
refused_by_limits(from, to) if {
	allowed := registry[from].may_call
	is_array(allowed)
	not permits(allowed, to)
}

refused_by_limits(from, to) if {
	allowed := registry[to].called_by
	is_array(allowed)
	not permits(allowed, from)
}

# valid_layers is true if both services have a layer that exists. SV202
# checks only such calls. SR001 and SV105 report a layer that is not valid.
valid_layers(from, to) if {
	layers[layer_of(from)]
	layers[layer_of(to)]
}

# METADATA
# title: Calls use known services
# description: The `from` and the `to` of each call are registered services or services in `services.new`.
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
# description: A service can call only the layers that its layer can call (see Layers).
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
		$"{c.from} ({from}) cannot call {c.to} ({to})",
		layer_fix(from, to, c.to),
	)
}

# METADATA
# title: Each new service has the suffix of its layer
# description: >-
#   The name of a new service ends with the suffix of its layer (Manager, Engine,
#   Accessor, or Resource). A client or a utility has no suffix.
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
		$"Change its name to {base_name(s.name)}{sfx} everywhere in the spec, or change its layer.",
	)
}

# METADATA
# title: A new service is not in the registry
# description: A service that is in the registry goes under `services.changed`, not under `services.new`.
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
# description: The `layer` of a new service is one of the layers.
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
# title: Each new service is in a call
# description: Each service in `services.new` is in one or more calls.
# custom:
#   id: SV106
deny contains f if {
	some i, s in new_list
	not used[s.name]
	f := lib.finding(
		rego.metadata.rule(),
		$"services.new[{i}]",
		$"{s.name} is in services.new, but no call uses it",
		$"Add the calls to or from {s.name}, or remove it from services.new.",
	)
}

# METADATA
# title: A call implements each acceptance criterion
# description: >-
#   The ID of each acceptance criterion is in the `covers` list of one or more calls.
#   Thus each acceptance criterion shows the services that implement it. In a spec
#   with no calls, the services in `services.changed` cover each criterion.
# custom:
#   id: SV107
deny contains f if {
	has_calls
	some i, ac in acceptance
	not covered[ac.id]
	f := lib.finding(
		rego.metadata.rule(),
		$"acceptance[{i}]",
		$"No call covers {ac.id}",
		$"Add {ac.id} to `covers` on the call(s) that implement it, or add the missing call.",
	)
}

# METADATA
# title: A spec with no calls names the services that it changes
# description: >-
#   A change inside one service adds no call between services. Such a spec has
#   `calls: []`, and it puts the service in `services.changed`. The services in
#   `services.changed` then cover each acceptance criterion.
# custom:
#   id: SV118
deny contains f if {
	not has_calls
	count(changed) == 0
	f := lib.finding(
		rego.metadata.rule(),
		"calls",
		"The spec has no calls and no service in services.changed",
		"Add the calls that the change adds. If the change is inside a service, add it to services.changed.",
	)
}

# METADATA
# title: Each call covers acceptance criteria of the spec
# description: Each ID in the `covers` list of a call is an acceptance criterion of the spec.
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
# title: Each changed service is in the registry
# description: Each name in `services.changed` is in the registry.
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
# title: Each new service has a different name
# description: Each name in `services.new` is there one time only.
# custom:
#   id: SV110
deny contains f if {
	some i, s in new_list
	repeated(i, s.name)
	f := lib.finding(
		rego.metadata.rule(),
		$"services.new[{i}].name",
		$"{s.name} is in services.new more than one time",
		"Remove this entry, or give the service a different name.",
	)
}

# METADATA
# title: Calls follow the limits in the registry
# description: >-
#   A registry entry can limit the calls of a service with `may_call`, and the
#   callers of a service with `called_by`. Each value is a layer or a service.
#   A limit makes the layer rules stricter. It cannot permit more calls.
# custom:
#   id: SV111
deny contains f if {
	some i, c in calls
	follows_layers(c)
	allowed := registry[c.from].may_call
	is_array(allowed)
	not permits(allowed, c.to)
	f := lib.finding(
		rego.metadata.rule(),
		$"calls[{i}]",
		$"{c.from} cannot call {c.to}, because may_call of {c.from} in the registry does not permit it",
		limit_fix(c.from, "can call only", allowed, "may_call", c.from),
	)
}

# METADATA
# title: Calls follow the limits in the registry
# description: >-
#   A registry entry can limit the calls of a service with `may_call`, and the
#   callers of a service with `called_by`. Each value is a layer or a service.
#   A limit makes the layer rules stricter. It cannot permit more calls.
# custom:
#   id: SV111
deny contains f if {
	some i, c in calls
	follows_layers(c)
	allowed := registry[c.to].called_by
	is_array(allowed)
	not permits(allowed, c.from)
	f := lib.finding(
		rego.metadata.rule(),
		$"calls[{i}]",
		$"{c.from} cannot call {c.to}, because called_by of {c.to} in the registry does not permit it",
		limit_fix(c.to, "can be called only by", allowed, "called_by", c.to),
	)
}

# METADATA
# title: Each removed call is in the registry
# description: >-
#   Each call in `removed_calls` is in the `calls` of its service in the registry.
# custom:
#   id: SV112
deny contains f if {
	some i, r in removed
	not r.to in registered_calls(r.from)
	f := lib.finding(
		rego.metadata.rule(),
		$"removed_calls[{i}]",
		$"{r.from} does not call {r.to} in the registry",
		$"Remove this item, or correct from and to. Use a call from the calls of {r.from} in the registry.",
	)
}

# METADATA
# title: Changed services take part in a call
# description: In a spec with calls, each service in `services.changed` is in one or more calls.
# custom:
#   id: SV201
warn contains f if {
	has_calls
	some i, name in changed
	registry[name]
	not used[name]
	f := lib.finding(
		rego.metadata.rule(),
		$"services.changed[{i}]",
		$"{name} is in services.changed, but no call uses it",
		$"Add the calls that change {name}, or remove it from services.changed.",
	)
}

# METADATA
# title: A changed service keeps only calls that the rules permit
# description: >-
#   The registry records the calls of each service today. For each service in
#   `services.changed`, each call that the spec does not remove follows the layer
#   rules and the limits. Thus a change does not keep a call that breaks the layers.
# custom:
#   id: SV202
warn contains f if {
	some i, name in changed
	some to in registered_calls(name)
	valid_layers(name, to)
	not [name, to] in removed_pairs
	not follows_layers({"from": name, "to": to})
	f := kept_call_finding(rego.metadata.rule(), i, name, to)
}

# METADATA
# title: A changed service keeps only calls that the rules permit
# description: >-
#   The registry records the calls of each service today. For each service in
#   `services.changed`, each call that the spec does not remove follows the layer
#   rules and the limits. Thus a change does not keep a call that breaks the layers.
# custom:
#   id: SV202
warn contains f if {
	some i, name in changed
	some to in registered_calls(name)
	valid_layers(name, to)
	not [name, to] in removed_pairs
	follows_layers({"from": name, "to": to})
	refused_by_limits(name, to)
	f := kept_call_finding(rego.metadata.rule(), i, name, to)
}

kept_call_finding(meta, i, name, to) := lib.finding(
	meta,
	$"services.changed[{i}]",
	$"{name} calls {to} in the registry, but {call_problem(name, to)}",
	concat(" ", array.concat(
		[
			$"This spec changes {name}, so fix the call.",
			$"Add \{ from: {name}, to: {to} } to removed_calls (a top-level list of the spec).",
			move_work(name),
		],
		relayer_hint(name, to),
	)),
)
