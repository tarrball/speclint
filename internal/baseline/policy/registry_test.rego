package specval.registry.dpl_test

import data.specval.registry.dpl
import rego.v1

test_a_valid_registry_has_no_findings if {
	project := {"services": {"OrderManager": {"layer": "manager"}, "CacheResource": {"layer": "resource"}}}
	count(dpl.deny) == 0 with data.project as project
}

test_no_registry_has_no_findings if {
	count(dpl.deny) == 0 with data.project as {}
}

test_sr001_unknown_or_missing_layer if {
	project := {"services": {
		"OrderAccessor": {"layer": "storage"},
		"NoLayerEngine": {},
	}}
	findings := dpl.deny with data.project as project
	{[f.rule, f.path] | some f in findings} == {
		["SR001", "services.OrderAccessor"],
		["SR001", "services.NoLayerEngine"],
	}
}

test_sr001_quotes_the_layer if {
	project := {"services": {"X": {"layer": "storage"}}}
	some f in dpl.deny with data.project as project
	f.msg == `X has no valid layer (got "storage")`
}

test_sr002_name_suffix if {
	project := {"services": {"PricingThing": {"layer": "engine"}}}
	some f in dpl.deny with data.project as project
	f.rule == "SR002"
	f.msg == "PricingThing is in the engine layer but does not end in Engine"
	f.fix == "Rename it to end in Engine, or fix its layer."
}
