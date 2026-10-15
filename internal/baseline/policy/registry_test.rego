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

test_sr001_layer_that_is_not_a_string if {
	project := {"services": {"X": {"layer": 7}}}
	some f in dpl.deny with data.project as project
	f.msg == "X has no valid layer (got 7)"
}

test_sr003_registry_must_be_a_mapping if {
	every services in [[], ["manager"], [{"layer": "manager"}], "x", 5] {
		findings := dpl.deny with data.project as {"services": services}
		{[f.rule, f.path] | some f in findings} == {["SR003", "services"]}
	}
}

test_sr003_missing_registry_is_not_an_error if {
	count(dpl.deny) == 0 with data.project as {}
}

test_sr002_name_suffix if {
	project := {"services": {"PricingThing": {"layer": "engine"}}}
	some f in dpl.deny with data.project as project
	f.rule == "SR002"
	f.msg == "PricingThing is in the engine layer but does not end in Engine"
	f.fix == "Change its name so that it ends in Engine, or change its layer."
}

test_sr004_entry_that_is_not_a_mapping if {
	project := {"services": {"OrderManager": "manager", "Odd": 7}}
	findings := dpl.deny with data.project as project
	{[f.rule, f.path, f.msg, f.fix] | some f in findings} == {
		[
			"SR004", "services.OrderManager", "OrderManager is not a mapping (got \"manager\")",
			"Write the entry as OrderManager: { layer: manager }.",
		],
		[
			"SR004", "services.Odd", "Odd is not a mapping (got 7)",
			"Write the entry as Odd: { layer: <layer> }, with one of: manager, engine, accessor, resource.",
		],
	}
}
