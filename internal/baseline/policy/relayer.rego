package specval.spec.dpl

# This file holds services.relayered: the moves of registered services to
# other layers, and the rules for them.

import data.specval.lib
import rego.v1

relayered := object.get(input, ["services", "relayered"], [])

# move_at maps each service that the spec moves to the index of its first
# entry in services.relayered. SV113 reports a service that is not
# registered, and SV116 reports a second entry for a service.
move_at[m.name] := i if {
	some i, m in relayered
	current[m.name]
	not moved_before(i, m.name)
}

# moved_before is true if an entry before index i of services.relayered has
# the name.
moved_before(i, name) if {
	some j, m in relayered
	j < i
	m.name == name
}

# renames maps the name today of each moved service to its new name. It holds
# only a new name that is free. SV117 reports the others.
renames[name] := m.rename if {
	some name, i in move_at
	m := relayered[i]
	free_name(i, m.rename)
}

# free_name is true if no other service has the new name, or can get it.
free_name(i, new) if {
	is_string(new)
	not taken[new]
	not renamed_twice(i, new)
}

# taken holds the names that a moved service cannot get: the registered
# services, the layers, and the new services.
taken contains name if some name, _ in current

taken contains name if some name, _ in layers

taken contains name if some name, _ in new_services

# renamed_twice is true if an entry other than index i has the new name.
renamed_twice(i, new) if {
	some j, m in relayered
	j != i
	m.rename == new
}

# new_name returns the name of a service after the moves.
new_name(name) := renames[name]

new_name(name) := name if not renames[name]

# moved_names holds the name after the move of each moved service.
moved_names contains new_name(name) if some name, _ in move_at

# moved_registry is the registry after the moves. A moved service has its new
# layer and its new name. Each list of names (calls, may_call, called_by) uses
# the new names.
moved_registry[new_name(name)] := moved_entry(name, s) if {
	some name, s in current
}

moved_entry(name, s) := object.union(with_new_names(s), {"layer": relayered[i].layer}) if {
	is_object(s)
	i := move_at[name]
} else := with_new_names(s) if is_object(s)

else := s

with_new_names(s) := object.union(s, {key: names |
	some key in ["calls", "may_call", "called_by"]
	is_array(s[key])
	names := [new_name(n) | some n in s[key]]
})

# breaks is true if the layer rules or the limits refuse the call.
breaks(from, to) if not follows_layers({"from": from, "to": to})

breaks(from, to) if {
	follows_layers({"from": from, "to": to})
	refused_by_limits(from, to)
}

# METADATA
# title: Each moved service is in the registry
# description: Each `name` in `services.relayered` is a registered service, with its name today.
# custom:
#   id: SV113
deny contains f if {
	is_object(current)
	names := concat(", ", sort(object.keys(current)))
	some i, m in relayered
	not current[m.name]
	f := lib.finding(
		rego.metadata.rule(),
		$"services.relayered[{i}].name",
		$"{m.name} is not a registered service",
		$"Use the name that a registered service has today: {names}. A new service goes under services.new.",
	)
}

# METADATA
# title: A moved service has the suffix of its new layer
# description: >-
#   The name of a moved service ends with the suffix of its new layer. If the
#   name today does not, `rename` gives the new name.
# custom:
#   id: SV114
deny contains f if {
	some name, i in move_at
	m := relayered[i]
	sfx := suffix[m.layer]
	final := object.get(m, "rename", name)
	not endswith(final, sfx)
	f := lib.finding(
		rego.metadata.rule(),
		$"services.relayered[{i}]",
		$"{final} moves to the {m.layer} layer but does not end in {sfx}",
		$"Set rename: {base_name(final)}{sfx} in this entry, and use the new name everywhere in the spec.",
	)
}

# METADATA
# title: Moved services use a known layer
# description: The `layer` of each entry in `services.relayered` is one of the layers.
# custom:
#   id: SV115
deny contains f if {
	some i, m in relayered
	not layers[m.layer]
	f := lib.finding(
		rego.metadata.rule(),
		$"services.relayered[{i}].layer",
		$"{m.layer} is not a layer",
		$"Use one of: {concat(", ", data.dpl.layer_order)}.",
	)
}

# METADATA
# title: A service moves one time only
# description: Each name in `services.relayered` is there one time only.
# custom:
#   id: SV116
deny contains f if {
	some i, m in relayered
	moved_before(i, m.name)
	f := lib.finding(
		rego.metadata.rule(),
		$"services.relayered[{i}].name",
		$"{m.name} is in services.relayered more than one time",
		"Remove this entry. Put the layer and the new name in the first entry.",
	)
}

# METADATA
# title: The new name of a moved service is free
# description: >-
#   The `rename` of a moved service is not the name of a registered service, a
#   new service, a layer, or the new name of another moved service.
# custom:
#   id: SV117
deny contains f if {
	some i, m in relayered
	new := m.rename
	is_string(new)
	not free_name(i, new)
	f := lib.finding(
		rego.metadata.rule(),
		$"services.relayered[{i}].rename",
		$"{new} is not free",
		"Use a name that no registered service, new service, layer, or other moved service has.",
	)
}

# METADATA
# title: A moved service keeps only calls that the rules permit
# description: >-
#   For each service in `services.relayered`, each registered call to or from
#   it that the spec does not remove follows the layer rules and the limits
#   after the move.
# custom:
#   id: SV203
warn contains f if {
	some name, i in move_at
	from := new_name(name)
	some to in registered_calls(from)
	valid_layers(from, to)
	not [from, to] in removed_pairs
	breaks(from, to)
	f := moved_call_finding(rego.metadata.rule(), i, from, to, from)
}

# METADATA
# title: A moved service keeps only calls that the rules permit
# description: >-
#   For each service in `services.relayered`, each registered call to or from
#   it that the spec does not remove follows the layer rules and the limits
#   after the move.
# custom:
#   id: SV203
warn contains f if {
	some name, i in move_at
	to := new_name(name)
	some from, _ in registry
	to in registered_calls(from)

	# SV202 and the rule above report the calls of changed and moved services.
	not from in changed
	not from in moved_names
	valid_layers(from, to)
	not [from, to] in removed_pairs
	breaks(from, to)
	f := moved_call_finding(rego.metadata.rule(), i, from, to, to)
}

moved_call_finding(meta, i, from, to, moved) := lib.finding(
	meta,
	$"services.relayered[{i}]",
	$"{from} calls {to} in the registry, but after the move {call_problem(from, to)}",
	concat(" ", [
		$"This spec moves {moved}, so fix the call.",
		$"Add \{ from: {from}, to: {to} } to removed_calls (a top-level list of the spec).",
		move_work(from),
	]),
)
