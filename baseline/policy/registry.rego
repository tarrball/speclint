package specval.registry.dpl

import data.specval.lib
import rego.v1

# Registry rules run once per `specval check`, not once per spec, and report
# against the data file that defines `services`.

registry := object.get(data.project, "services", {})

# METADATA
# title: Registered services use a known layer
# custom:
#   id: SR001
deny contains f if {
	some name, s in registry
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
		$"Rename it to end in {sfx}, or fix its layer.",
	)
}
