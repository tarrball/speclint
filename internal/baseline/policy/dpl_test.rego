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
	endswith(f.fix, "Otherwise use a registered service: PricingEngine (engine).")
}

test_sv101_suggests_all_services_without_a_layer_suffix if {
	s := spec({"calls": [call("Somebody", "OrderAccessor")]})
	every_service := "BillingAccessor (accessor), OrderAccessor (accessor), OrderManager (manager), PricingEngine (engine)"
	some f in dpl.deny with input as s with data.project as project
	f.rule == "SV101"
	endswith(f.fix, $"{every_service}.")
}

test_sv102_layer_rules if {
	s := spec({"calls": [
		call("OrderManager", "PricingEngine"), # allowed
		call("PricingEngine", "OrderAccessor"), # allowed
		call("PricingEngine", "OrderManager"), # engines cannot call managers
		call("OrderAccessor", "PricingEngine"), # accessors cannot call engines
		call("OrderManager", "OrderManager"), # managers cannot call managers
	]})
	findings := dpl.deny with input as s with data.project as project
	{f.path | some f in findings; f.rule == "SV102"} == {"calls[2]", "calls[3]", "calls[4]"}
}

test_sv102_fix_names_the_layers_that_can_make_the_call if {
	s := spec({"calls": [call("OrderAccessor", "PricingEngine")]})
	some f in dpl.deny with input as s with data.project as project
	f.rule == "SV102"
	f.fix == "Accessors can call only resources and utilities. Send the call to PricingEngine through a manager."
}

test_sv102_fix_for_a_call_up_to_a_manager if {
	s := spec({"calls": [call("PricingEngine", "OrderManager")]})
	some f in dpl.deny with input as s with data.project as project
	f.rule == "SV102"
	f.fix == "Engines can call only accessors and utilities. Send the call to OrderManager through a client."
}

test_sv102_fix_for_a_layer_that_no_one_calls if {
	s := spec({
		"services": {"new": [new_service("OrderApi", "client")]},
		"calls": [call("OrderApi", "OrderManager"), call("OrderManager", "OrderApi")],
	})
	some f in dpl.deny with input as s with data.project as project
	f.rule == "SV102"
	f.fix == "Managers can call only engines, accessors and utilities. No service can call clients."
}

test_sv102_fix_for_a_resource if {
	s := spec({
		"services": {"new": [new_service("CacheResource", "resource")]},
		"calls": [call("CacheResource", "OrderAccessor")],
	})
	some f in dpl.deny with input as s with data.project as project
	f.rule == "SV102"
	startswith(f.fix, "Resources can call only utilities.")
}

test_clients_call_managers_and_utilities if {
	s := spec({
		"services": {"new": [new_service("OrderApi", "client"), new_service("Logging", "utility")]},
		"calls": [
			call("OrderApi", "OrderManager"), # correct
			call("OrderApi", "Logging"), # correct
			call("OrderApi", "PricingEngine"), # a client cannot call an engine
			call("OrderApi", "OrderAccessor"), # a client cannot call an accessor
		],
	})
	findings := dpl.deny with input as s with data.project as project
	{[f.rule, f.path] | some f in findings} == {["SV102", "calls[2]"], ["SV102", "calls[3]"]}
}

test_each_layer_calls_utilities if {
	s := spec({
		"services": {"new": [
			new_service("Logging", "utility"),
			new_service("Config", "utility"),
			new_service("CacheResource", "resource"),
		]},
		"calls": [
			call("OrderManager", "Logging"),
			call("PricingEngine", "Logging"),
			call("OrderAccessor", "Logging"),
			call("CacheResource", "Logging"),
			call("Logging", "Config"),
			call("OrderManager", "CacheResource"), # a manager cannot call a resource
			call("Logging", "PricingEngine"), # a utility can call only utilities
		],
	})
	findings := dpl.deny with input as s with data.project as project
	{[f.rule, f.path] | some f in findings} == {["SV102", "calls[5]"], ["SV102", "calls[6]"]}
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
		"Change its name to RefundCalcEngine everywhere in the spec, or change its layer.",
		"Change its name to TaxEngine everywhere in the spec, or change its layer.",
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
	f.fix == "Use one of: client, manager, engine, accessor, resource, utility."
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

test_sv110_a_new_service_name_two_times if {
	# The second entry has another layer. The other rules use the first
	# entry, so the call from the manager to the engine is correct.
	s := spec({
		"services": {"new": [new_service("RefundEngine", "engine"), new_service("RefundEngine", "accessor")]},
		"calls": [call("OrderManager", "RefundEngine")],
	})
	findings := dpl.deny with input as s with data.project as project
	{[f.rule, f.path] | some f in findings} == {
		["SV110", "services.new[1].name"],
		["SV103", "services.new[1].name"],
	}
}

# limited is a registry with limits for some services.
limited := {"services": {
	"OrderManager": {"layer": "manager"},
	"BillingManager": {"layer": "manager", "may_call": ["accessor", "LogUtility"]},
	"PricingEngine": {"layer": "engine", "called_by": ["OrderManager"]},
	"OrderAccessor": {"layer": "accessor"},
	"ConfigUtility": {"layer": "utility", "may_call": []},
	"LogUtility": {"layer": "utility"},
	"AuditUtility": {"layer": "utility", "called_by": ["manager"]},
}}

test_sv111_calls_that_the_limits_permit if {
	s := spec({"calls": [
		call("OrderManager", "PricingEngine"), # PricingEngine: called_by OrderManager
		call("LogUtility", "ConfigUtility"), # the may_call of ConfigUtility limits only its own calls
		call("BillingManager", "OrderAccessor"), # a layer in may_call
		call("BillingManager", "LogUtility"), # a service in may_call
		call("OrderManager", "AuditUtility"), # a layer in called_by
	]})
	count([f | some f in dpl.deny with input as s with data.project as limited; f.rule == "SV111"]) == 0
}

test_sv111_calls_that_the_limits_refuse if {
	s := spec({"calls": [
		call("ConfigUtility", "LogUtility"), # an empty may_call
		call("BillingManager", "PricingEngine"), # not in may_call, and not in called_by
		call("LogUtility", "AuditUtility"), # a utility is not in called_by
		call("BillingManager", "ConfigUtility"), # not in may_call
	]})
	paths := {f.path | some f in dpl.deny with input as s with data.project as limited; f.rule == "SV111"}
	paths == {"calls[0]", "calls[1]", "calls[2]", "calls[3]"}
}

test_sv111_fixes if {
	s := spec({"calls": [call("BillingManager", "PricingEngine"), call("ConfigUtility", "LogUtility")]})
	fixes := {f.fix | some f in dpl.deny with input as s with data.project as limited; f.rule == "SV111"}
	owners := "ask the owners of the registry to change"
	fixes == {
		$"BillingManager can call only accessor or LogUtility. Change the call, or {owners} may_call of BillingManager.",
		$"PricingEngine can be called only by OrderManager. Change the call, or {owners} called_by of PricingEngine.",
		$"ConfigUtility has an empty may_call. Remove the call, or {owners} may_call of ConfigUtility.",
	}
}

test_sv111_does_not_repeat_sv102 if {
	# The layer rules refuse a call from a utility to a manager, so only SV102 reports it.
	s := spec({"calls": [call("ConfigUtility", "OrderManager")]})
	{f.rule | some f in dpl.deny with input as s with data.project as limited} == {"SV102"}
}

test_sv111_new_services_have_no_limits if {
	s := spec({
		"services": {"new": [new_service("TaxEngine", "engine")]},
		"calls": [call("OrderManager", "TaxEngine"), call("TaxEngine", "OrderAccessor")],
	})
	count(dpl.deny) == 0 with input as s with data.project as limited
}

test_sv111_the_name_of_a_layer_means_the_layer if {
	# The service "manager" has the name of a layer (SR007). In the limits,
	# "manager" is the layer, so SV111 refuses both calls, as SR006 expects.
	project := {"services": {
		"manager": {"layer": "utility"},
		"ConfigUtility": {"layer": "utility", "may_call": ["manager"]},
		"AuditUtility": {"layer": "utility", "called_by": ["manager"]},
	}}
	s := spec({"calls": [call("ConfigUtility", "manager"), call("manager", "AuditUtility")]})
	paths := {f.path | some f in dpl.deny with input as s with data.project as project; f.rule == "SV111"}
	paths == {"calls[0]", "calls[1]"}
}

# today is a registry with the calls of each service today. HookCommand, a
# client, calls an accessor, which the layer rules do not permit.
today := {"services": {
	"HookCommand": {"layer": "client", "calls": ["ConfigAccessor", "CheckManager"]},
	"CheckManager": {"layer": "manager", "calls": ["ConfigAccessor"]},
	"ConfigAccessor": {"layer": "accessor"},
	"AuditUtility": {"layer": "utility", "called_by": ["manager"]},
	"LogUtility": {"layer": "utility", "calls": ["AuditUtility"]},
}}

hook_spec(changes) := spec(object.union(
	{
		"services": {"changed": ["HookCommand"]},
		"calls": [call("HookCommand", "CheckManager")],
	},
	changes,
))

test_sv202_a_changed_service_keeps_a_call_that_breaks_the_layers if {
	findings := dpl.warn with input as hook_spec({}) with data.project as today
	{[f.rule, f.path, f.msg] | some f in findings} == {[
		"SV202", "services.changed[0]",
		"HookCommand calls ConfigAccessor in the registry, but clients cannot call accessors",
	]}
	some f in findings
	f.fix == concat(" ", [
		"This spec changes HookCommand, so fix the call.",
		"Add { from: HookCommand, to: ConfigAccessor } to removed_calls (a top-level list of the spec).",
		"Then move the work to a manager or utility that HookCommand can call.",
	])
}

test_sv202_a_removed_call_is_not_reported if {
	s := hook_spec({"removed_calls": [{"from": "HookCommand", "to": "ConfigAccessor"}]})
	count(dpl.warn) == 0 with input as s with data.project as today
	count(dpl.deny) == 0 with input as s with data.project as today
}

test_sv202_a_service_that_is_not_changed_is_not_checked if {
	# HookCommand keeps its call, but the spec does not change HookCommand.
	s := spec({"calls": [call("HookCommand", "CheckManager")]})
	count(dpl.warn) == 0 with input as s with data.project as today
}

test_sv202_a_kept_call_that_a_limit_refuses if {
	s := spec({"services": {"changed": ["LogUtility"]}, "calls": [call("LogUtility", "AuditUtility")]})
	{[f.rule, f.msg] | some f in dpl.warn with input as s with data.project as today} == {[
		"SV202",
		"LogUtility calls AuditUtility in the registry, but a limit in the registry refuses it",
	]}
}

test_sv112_a_removed_call_that_is_not_in_the_registry if {
	s := hook_spec({"removed_calls": [
		{"from": "HookCommand", "to": "ConfigAccessor"},
		{"from": "CheckManager", "to": "HookCommand"},
		{"from": "Nobody", "to": "ConfigAccessor"},
	]})
	paths := {f.path | some f in dpl.deny with input as s with data.project as today; f.rule == "SV112"}
	paths == {"removed_calls[1]", "removed_calls[2]"}
}

test_sv201_a_service_in_removed_calls_is_used if {
	# HookCommand is only in removed_calls. The spec moves the work to
	# CheckManager, so HookCommand takes part in the change.
	s := spec({
		"services": {"changed": ["HookCommand"]},
		"calls": [call("CheckManager", "ConfigAccessor")],
		"removed_calls": [{"from": "HookCommand", "to": "ConfigAccessor"}],
	})
	count(dpl.warn) == 0 with input as s with data.project as today
}

test_sv202_a_service_without_a_valid_layer_is_not_checked if {
	# SR001 reports the layer. SV202 must not tell that a limit refuses the call.
	project := {"services": {
		"HookCommand": {"calls": ["ConfigAccessor"]},
		"NumberCommand": {"layer": 7, "calls": ["ConfigAccessor"]},
		"ConfigAccessor": {"layer": "accessor"},
		"OrderManager": {"layer": "manager"},
	}}
	s := spec({"services": {"changed": ["HookCommand", "NumberCommand"]}})
	count([f | some f in dpl.warn with input as s with data.project as project; f.rule == "SV202"]) == 0
}
