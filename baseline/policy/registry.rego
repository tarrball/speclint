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
		sprintf("services.%s", [name]),
		sprintf("%s has no valid layer (got %q)", [name, layer]),
		sprintf("Set layer to one of: %s.", [concat(", ", data.dpl.layer_order)]),
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
		sprintf("services.%s", [name]),
		sprintf("%s is in the %s layer but does not end in %s", [name, s.layer, sfx]),
		sprintf("Rename it to end in %s, or fix its layer.", [sfx]),
	)
}
