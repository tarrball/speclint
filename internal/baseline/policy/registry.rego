package specval.registry.dpl

import data.specval.lib
import rego.v1

# specval runs the registry rules one time for each check, not one time for
# each spec. Their findings refer to the data file that defines `services`.

registry := object.get(data.project, "services", {})

# entries is the registry if it is a mapping. If it is not, SR003 reports it,
# and the other rules have no entries to check.
default entries := {}

entries := registry if is_object(registry)

# The keys of a registry entry that limit the calls of a service.
limit_keys := ["may_call", "called_by"]

# limit returns the list at key in the entry s. It is empty if the entry has
# no list at key. SR005 reports a value that is not a list.
limit(s, key) := list if {
	list := s[key]
	is_array(list)
} else := []

# layer_name returns x if x is a layer, or else the layer of the service x.
layer_name(x) := x if data.dpl.layers[x]

else := entries[x].layer

# METADATA
# title: The registry is a mapping
# description: data.project.services maps each service name to its layer.
# custom:
#   id: SR003
deny contains f if {
	not is_object(registry)
	f := lib.finding(
		rego.metadata.rule(),
		"services",
		$"services is not a mapping (type: {type_name(registry)})",
		"Write services as a mapping from each service name to its layer, for example OrderManager: { layer: manager }.",
	)
}

# METADATA
# title: Registered services use a known layer
# custom:
#   id: SR001
deny contains f if {
	some name, s in entries
	is_object(s)
	layer := object.get(s, "layer", "")
	not data.dpl.layers[layer]
	f := lib.finding(
		rego.metadata.rule(),
		$"services.{name}",
		$"{name} has no valid layer (got {lib.quote(layer)})",
		$"Set layer to one of: {concat(", ", data.dpl.layer_order)}.",
	)
}

# METADATA
# title: Each registered service has the suffix of its layer
# custom:
#   id: SR002
deny contains f if {
	some name, s in entries
	sfx := data.dpl.suffix[s.layer]
	not endswith(name, sfx)
	f := lib.finding(
		rego.metadata.rule(),
		$"services.{name}",
		$"{name} is in the {s.layer} layer but does not end in {sfx}",
		$"Change its name so that it ends in {sfx}, or change its layer.",
	)
}

# METADATA
# title: A service does not have the name of a layer
# description: >-
#   The name of a registered service is not the name of a layer. In `may_call` and
#   `called_by`, the name of a layer always means the layer.
# custom:
#   id: SR007
deny contains f if {
	some name, _ in entries
	data.dpl.layers[name]
	f := lib.finding(
		rego.metadata.rule(),
		$"services.{name}",
		$"{name} is the name of a layer",
		"Give the service a different name, for example a name in PascalCase.",
	)
}

# METADATA
# title: Each registry entry is a mapping
# description: Each service in the registry maps to an entry that has a layer.
# custom:
#   id: SR004
deny contains f if {
	some name, s in entries
	not is_object(s)
	f := lib.finding(
		rego.metadata.rule(),
		$"services.{name}",
		$"{name} is not a mapping (got {lib.quote(s)})",
		entry_fix(name, s),
	)
}

# METADATA
# title: Each limit is a list of layers and registered services
# description: >-
#   `may_call` and `called_by` in a registry entry are lists. Each value is a
#   layer or a registered service.
# custom:
#   id: SR005
deny contains f if {
	some name, s in entries
	is_object(s)
	some key in limit_keys
	list := s[key]
	not is_array(list)
	f := lib.finding(
		rego.metadata.rule(),
		$"services.{name}.{key}",
		$"{key} of {name} is not a list (got {lib.quote(list)})",
		$"Write {key} as a list of layers and services, for example [manager, OrderManager].",
	)
}

# METADATA
# title: Each limit is a list of layers and registered services
# description: >-
#   `may_call` and `called_by` in a registry entry are lists. Each value is a
#   layer or a registered service.
# custom:
#   id: SR005
deny contains f if {
	some name, s in entries
	is_object(s)
	some key in limit_keys
	some j, x in limit(s, key)
	not data.dpl.layers[x]
	not entries[x]
	f := lib.finding(
		rego.metadata.rule(),
		$"services.{name}.{key}[{j}]",
		$"{lib.quote(x)} in {key} of {name} is not a layer or a registered service",
		$"Use a layer ({concat(", ", data.dpl.layer_order)}) or a registered service.",
	)
}

# METADATA
# title: A limit does not permit more than the layer rules
# description: >-
#   A value in `may_call` or `called_by` names a layer or a service that the
#   layer rules permit. A limit can only make the layer rules stricter.
# custom:
#   id: SR006
warn contains f if {
	some name, s in entries
	from := s.layer
	some j, x in limit(s, "may_call")
	to := layer_name(x)
	not to in object.get(data.dpl.layers, from, [])
	f := limit_warning(rego.metadata.rule(), name, "may_call", j, x, from, to)
}

# METADATA
# title: A limit does not permit more than the layer rules
# description: >-
#   A value in `may_call` or `called_by` names a layer or a service that the
#   layer rules permit. A limit can only make the layer rules stricter.
# custom:
#   id: SR006
warn contains f if {
	some name, s in entries
	to := s.layer
	some j, x in limit(s, "called_by")
	from := layer_name(x)
	not to in object.get(data.dpl.layers, from, [])
	f := limit_warning(rego.metadata.rule(), name, "called_by", j, x, from, to)
}

limit_warning(meta, name, key, j, x, from, to) := lib.finding(
	meta,
	$"services.{name}.{key}[{j}]",
	$"{key} of {name} has {x}, but the layer rules do not permit a call from a {from} to a {to}",
	$"Remove {x} from {key}. A limit can only make the layer rules stricter.",
)

# entry_fix tells how to write a registry entry. If the entry is the name of
# a layer, the fix uses it.
entry_fix(name, s) := $"Write the entry as {name}: \{ layer: {s} }." if data.dpl.layers[s]

else := $"Write the entry as {name}: \{ layer: <layer> }, with one of: {concat(", ", data.dpl.layer_order)}."
