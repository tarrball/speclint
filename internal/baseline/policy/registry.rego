package specval.registry.dpl

import data.specval.lib
import rego.v1

# specval runs the registry rules one time for each check, not one time for
# each spec. Their findings refer to the data file that defines `services`.

registry := object.get(data.project, "services", {})

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
	some name, s in registry
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
# title: Registered services are named for their layer
# custom:
#   id: SR002
deny contains f if {
	some name, s in registry
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
# title: Each registry entry is a mapping
# description: Each service in the registry maps to an entry that has a layer.
# custom:
#   id: SR004
deny contains f if {
	some name, s in registry
	not is_object(s)
	f := lib.finding(
		rego.metadata.rule(),
		$"services.{name}",
		$"{name} is not a mapping (got {lib.quote(s)})",
		entry_fix(name, s),
	)
}

# entry_fix tells how to write a registry entry. If the entry is the name of
# a layer, the fix uses it.
entry_fix(name, s) := $"Write the entry as {name}: \{ layer: {s} }." if data.dpl.layers[s]

else := $"Write the entry as {name}: \{ layer: <layer> }, with one of: {concat(", ", data.dpl.layer_order)}."
