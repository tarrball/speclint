package main

import rego.v1

suffix := {"manager": "Manager", "engine": "Engine", "accessor": "Accessor", "resource": "Resource"}

# Every name in the spec must be in the registry
deny contains msg if {
	some edge in input.calls
	some name in [edge.from, edge.to]
	not data.services[name]
	msg := sprintf("%s is not a registered service", [name])
}

# Every call must be allowed by the layer rules
deny contains msg if {
	some edge in input.calls
	from := data.services[edge.from].layer
	to := data.services[edge.to].layer
	not to in data.layers[from]
	msg := sprintf("%s (%s) may not call %s (%s)", [edge.from, from, edge.to, to])
}

# Registry entries must match the naming convention
deny contains msg if {
	some name, svc in data.services
	not endswith(name, suffix[svc.layer])
	msg := sprintf("%s is registered as %s but lacks the %s suffix", [name, svc.layer, suffix[svc.layer]])
}
