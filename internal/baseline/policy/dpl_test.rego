package specval.spec.dpl_test

import data.specval.spec.dpl
import rego.v1

project := {"services": {
	"OrderManager": {"layer": "manager"},
	"PricingEngine": {"layer": "engine"},
	"OrderAccessor": {"layer": "accessor"},
	"BillingAccessor": {"layer": "accessor"},
}}

# spec returns a valid spec with the fields in changes replaced.
spec(changes) := object.union(
	{
		"specval": 1,
		"id": "A-1",
		"kind": "feature",
		"title": "T",
		"summary": "S",
		"acceptance": [{"id": "AC1", "text": "T"}],
		"calls": [{"from": "OrderManager", "to": "OrderAccessor", "op": "Get", "covers": ["AC1"]}],
	},
	changes,
)

call(from, to) := {"from": from, "to": to, "op": "Do", "covers": ["AC1"]}

new_service(name, layer) := {"name": name, "layer": layer, "purpose": "P"}

test_a_valid_spec_has_no_findings if {
	count(dpl.deny) == 0 with input as spec({}) with data.project as project
	count(dpl.warn) == 0 with input as spec({}) with data.project as project
}

test_sv101_unknown_services if {
	s := spec({"calls": [call("Nobody", "OrderAccessor"), call("OrderManager", "GhostEngine")]})
	findings := dpl.deny with input as s with data.project as project
	{f.path | some f in findings; f.rule == "SV101"} == {"calls[0].from", "calls[1].to"}
}

test_sv101_new_services_are_known if {
	s := spec({
		"services": {"new": [new_service("RefundEngine", "engine")]},
		"calls": [call("OrderManager", "RefundEngine")],
	})
	count(dpl.deny) == 0 with input as s with data.project as project
}

test_sv101_suggests_services_of_the_same_layer if {
	s := spec({"calls": [call("OrderManager", "GhostEngine")]})
	some f in dpl.deny with input as s with data.project as project
	f.rule == "SV101"
	endswith(f.fix, "Otherwise use a registered service: PricingEngine.")
}

test_sv101_suggests_all_services_without_a_layer_suffix if {
	s := spec({"calls": [call("Somebody", "OrderAccessor")]})
	some f in dpl.deny with input as s with data.project as project
	f.rule == "SV101"
	endswith(f.fix, "BillingAccessor, OrderAccessor, OrderManager, PricingEngine.")
}

test_sv102_layer_rules if {
	s := spec({"calls": [
		call("OrderManager", "PricingEngine"), # allowed
		call("PricingEngine", "OrderAccessor"), # allowed
		call("PricingEngine", "OrderManager"), # engines may not call managers
		call("OrderAccessor", "PricingEngine"), # accessors may not call engines
		call("OrderManager", "OrderManager"), # managers may not call managers
	]})
	findings := dpl.deny with input as s with data.project as project
	{f.path | some f in findings; f.rule == "SV102"} == {"calls[2]", "calls[3]", "calls[4]"}
}

test_sv102_fix_names_the_layers_that_can_make_the_call if {
	s := spec({"calls": [call("OrderAccessor", "PricingEngine")]})
	some f in dpl.deny with input as s with data.project as project
	f.rule == "SV102"
	f.fix == "Accessors may call only resources. Route the call to PricingEngine through a manager instead."
}

test_sv102_fix_for_a_layer_that_no_one_calls if {
	s := spec({"calls": [call("PricingEngine", "OrderManager")]})
	some f in dpl.deny with input as s with data.project as project
	f.rule == "SV102"
	f.fix == "Engines may call only accessors. Managers are entry points; no service calls them."
}

test_sv102_fix_for_a_layer_that_calls_nothing if {
	s := spec({
		"services": {"new": [new_service("CacheResource", "resource")]},
		"calls": [call("CacheResource", "OrderAccessor")],
	})
	some f in dpl.deny with input as s with data.project as project
	f.rule == "SV102"
	startswith(f.fix, "Resources may not call other services.")
}

test_sv102_uses_the_layer_of_a_new_service if {
	s := spec({
		"services": {"new": [new_service("RefundEngine", "engine")]},
		"calls": [call("OrderManager", "RefundEngine"), call("RefundEngine", "PricingEngine")],
	})
	findings := dpl.deny with input as s with data.project as project
	{f.path | some f in findings; f.rule == "SV102"} == {"calls[1]"}
}

test_sv102_skips_a_service_with_an_unknown_layer if {
	# SV105 reports the unknown layer. SV102 cannot apply.
	s := spec({
		"services": {"new": [new_service("AuditHelper", "helper")]},
		"calls": [call("AuditHelper", "OrderAccessor")],
	})
	findings := dpl.deny with input as s with data.project as project
	{f.rule | some f in findings} == {"SV105"}
}

test_sv102_skips_a_registered_layer_that_is_not_a_string if {
	# SR001 reports the layer. SV102 cannot apply.
	registry := {"services": object.union(project.services, {"NumberEngine": {"layer": 7}})}
	s := spec({"calls": [call("OrderManager", "NumberEngine")]})
	count(dpl.deny) == 0 with input as s with data.project as registry
}

test_sv103_name_suffix if {
	s := spec({
		"services": {"new": [new_service("RefundCalc", "engine"), new_service("TaxManager", "engine")]},
		"calls": [call("OrderManager", "RefundCalc"), call("OrderManager", "TaxManager")],
	})
	findings := dpl.deny with input as s with data.project as project
	fixes := {f.fix | some f in findings; f.rule == "SV103"}
	fixes == {
		"Rename it to RefundCalcEngine everywhere in the spec, or change its layer.",
		"Rename it to TaxEngine everywhere in the spec, or change its layer.",
	}
}

test_sv104_registered_service_declared_as_new if {
	s := spec({
		"services": {"new": [new_service("PricingEngine", "engine")]},
		"calls": [call("OrderManager", "PricingEngine")],
	})
	findings := dpl.deny with input as s with data.project as project
	{[f.rule, f.path] | some f in findings} == {["SV104", "services.new[0].name"]}
}

test_sv104_layer_that_is_not_a_string if {
	registry := {"services": object.union(project.services, {"NumberEngine": {"layer": 7}})}
	s := spec({
		"services": {"new": [new_service("NumberEngine", "engine")]},
		"calls": [call("OrderManager", "NumberEngine")],
	})
	some f in dpl.deny with input as s with data.project as registry
	f.rule == "SV104"
	f.msg == "NumberEngine is already registered (7 layer)"
}

test_sv105_unknown_layer if {
	s := spec({
		"services": {"new": [new_service("X", "helper")]},
		"calls": [call("OrderManager", "X")],
	})
	some f in dpl.deny with input as s with data.project as project
	f.rule == "SV105"
	f.path == "services.new[0].layer"
	f.fix == "Use one of: manager, engine, accessor, resource."
}

test_sv106_unused_new_service if {
	s := spec({"services": {"new": [new_service("RefundEngine", "engine")]}})
	findings := dpl.deny with input as s with data.project as project
	{[f.rule, f.path] | some f in findings} == {["SV106", "services.new[0]"]}
}

test_sv107_acceptance_criterion_without_a_call if {
	s := spec({"acceptance": [{"id": "AC1", "text": "T"}, {"id": "AC2", "text": "T"}]})
	findings := dpl.deny with input as s with data.project as project
	{[f.rule, f.path] | some f in findings} == {["SV107", "acceptance[1]"]}
}

test_sv108_call_covers_an_unknown_criterion if {
	s := spec({"calls": [{"from": "OrderManager", "to": "OrderAccessor", "op": "Get", "covers": ["AC1", "AC9"]}]})
	some f in dpl.deny with input as s with data.project as project
	f.rule == "SV108"
	f.path == "calls[0].covers[1]"
	f.fix == "Use one of: AC1, or add AC9 under acceptance."
}

test_sv109_changed_service_not_registered if {
	s := spec({"services": {"changed": ["OrderManager", "ShippingManager"]}})
	findings := dpl.deny with input as s with data.project as project
	{[f.rule, f.path] | some f in findings} == {["SV109", "services.changed[1]"]}
}

test_sv201_changed_service_not_in_a_call if {
	s := spec({"services": {"changed": ["OrderManager", "BillingAccessor"]}})
	count(dpl.deny) == 0 with input as s with data.project as project
	findings := dpl.warn with input as s with data.project as project
	{[f.rule, f.path] | some f in findings} == {["SV201", "services.changed[1]"]}
}

test_sv201_does_not_repeat_sv109 if {
	s := spec({"services": {"changed": ["GhostManager"]}})
	count(dpl.warn) == 0 with input as s with data.project as project
}

test_findings_have_the_rule_title if {
	s := spec({"calls": [call("Nobody", "OrderAccessor")]})
	some f in dpl.deny with input as s with data.project as project
	f.title == "Calls use known services"
}

test_no_registry if {
	findings := dpl.deny with input as spec({}) with data.project as {}
	{[f.rule, f.path] | some f in findings} == {["SV101", "calls[0].from"], ["SV101", "calls[0].to"]}
}
