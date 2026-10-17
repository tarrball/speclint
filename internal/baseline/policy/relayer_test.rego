package specval.spec.dpl_test

import data.specval.spec.dpl
import rego.v1

# before is a registry where ClaudeAccessor holds a protocol, not stored data.
before := {"services": {
	"HookCommand": {"layer": "client", "calls": ["ClaudeAccessor"]},
	"CheckManager": {"layer": "manager", "calls": ["ClaudeAccessor"]},
	"ClaudeAccessor": {"layer": "accessor", "calls": ["StoreResource"]},
	"StoreResource": {"layer": "resource"},
	"LogUtility": {"layer": "utility", "may_call": ["ClaudeAccessor"]},
	"OrderManager": {"layer": "manager", "calls": ["PriceAccessor"]},
	"PriceAccessor": {"layer": "accessor"},
}}

to_hook := {"name": "ClaudeAccessor", "layer": "utility", "rename": "ClaudeHook", "reason": "R"}

move_spec(moves, changes) := spec(object.union(
	{
		"services": {"relayered": moves},
		"calls": [call("HookCommand", "ClaudeHook"), call("LogUtility", "ClaudeHook")],
		"removed_calls": [{"from": "ClaudeHook", "to": "StoreResource"}],
	},
	changes,
))

rules_of(findings) := {f.rule | some f in findings}

test_a_move_checks_the_calls_with_the_new_layer_and_name if {
	s := move_spec([to_hook], {})
	count(dpl.deny) == 0 with input as s with data.project as before
	count(dpl.warn) == 0 with input as s with data.project as before

	# Without the move, the client cannot call the accessor.
	old := spec({"calls": [call("HookCommand", "ClaudeAccessor")]})
	rules_of(dpl.deny) == {"SV102"} with input as old with data.project as before
}

test_a_call_with_the_old_name_gets_the_new_name if {
	s := move_spec([to_hook], {"calls": [call("HookCommand", "ClaudeAccessor")]})
	some f in dpl.deny with input as s with data.project as before
	f.rule == "SV101"
	f.fix == "This spec renames ClaudeAccessor to ClaudeHook in services.relayered. Use ClaudeHook."
}

test_sv114_a_name_without_the_suffix_of_the_new_layer if {
	moves := [
		{"name": "PriceAccessor", "layer": "engine", "reason": "R"},
		{"name": "ClaudeAccessor", "layer": "engine", "rename": "ClaudeCalc", "reason": "R"},
	]
	s := spec({"services": {"relayered": moves}})
	fixes := {f.path: f.fix | some f in dpl.deny with input as s with data.project as before; f.rule == "SV114"}
	fixes == {
		"services.relayered[0]": "Set rename: PriceEngine in this entry, and use the new name everywhere in the spec.",
		"services.relayered[1]": "Set rename: ClaudeCalcEngine in this entry, and use the new name everywhere in the spec.",
	}
}

test_sv203_a_kept_call_from_a_moved_service if {
	s := move_spec([to_hook], {"removed_calls": []})
	findings := dpl.warn with input as s with data.project as before
	{[f.rule, f.path, f.msg] | some f in findings} == {[
		"SV203", "services.relayered[0]",
		"ClaudeHook calls StoreResource in the registry, but after the move utilities cannot call resources",
	]}
}

test_sv203_a_kept_call_to_a_moved_service if {
	move := {"name": "PriceAccessor", "layer": "resource", "rename": "PriceResource", "reason": "R"}
	s := spec({"services": {"relayered": [move]}})
	findings := dpl.warn with input as s with data.project as before
	{[f.rule, f.msg] | some f in findings} == {[
		"SV203",
		"OrderManager calls PriceResource in the registry, but after the move managers cannot call resources",
	]}
	some f in findings
	startswith(f.fix, concat(" ", [
		"This spec moves PriceResource, so fix the call.",
		"Add { from: OrderManager, to: PriceResource } to removed_calls",
	]))
}

test_sv203_leaves_a_changed_moved_service_to_sv202 if {
	# The spec moves and changes ClaudeHook, and keeps its call to
	# StoreResource. The call gets one finding.
	s := move_spec([to_hook], {"removed_calls": [], "services": {"relayered": [to_hook], "changed": ["ClaudeHook"]}})
	found := {[f.rule, f.path] | some f in dpl.warn with input as s with data.project as before}
	found == {["SV202", "services.changed[0]"]}
}

test_sv203_leaves_a_changed_caller_to_sv202 if {
	move := {"name": "PriceAccessor", "layer": "resource", "rename": "PriceResource", "reason": "R"}
	changed := spec({"services": {"relayered": [move], "changed": ["OrderManager"]}})
	rules_of(dpl.warn) == {"SV202"} with input as changed with data.project as before
}

test_sv113_sv115_sv116_sv117_bad_moves if {
	moves := [
		{"name": "GhostAccessor", "layer": "utility", "reason": "R"},
		{"name": "PriceAccessor", "layer": "helper", "reason": "R"},
		{"name": "PriceAccessor", "layer": "utility", "reason": "R"},
		{"name": "ClaudeAccessor", "layer": "manager", "rename": "OrderManager", "reason": "R"},
		{"name": "StoreResource", "layer": "manager", "rename": "manager", "reason": "R"},
		{"name": "LogUtility", "layer": "utility", "rename": "Twice", "reason": "R"},
		{"name": "HookCommand", "layer": "client", "rename": "Twice", "reason": "R"},
	]
	s := spec({"services": {"relayered": moves}})
	found := {[f.rule, f.path] |
		some f in dpl.deny with input as s with data.project as before
		f.rule in {"SV113", "SV115", "SV116", "SV117"}
	}
	found == {
		["SV113", "services.relayered[0].name"],
		["SV115", "services.relayered[1].layer"],
		["SV116", "services.relayered[2].name"],
		["SV117", "services.relayered[3].rename"],
		["SV117", "services.relayered[4].rename"],
		["SV117", "services.relayered[5].rename"],
		["SV117", "services.relayered[6].rename"],
	}
}

test_a_move_with_a_registry_that_is_not_a_mapping if {
	# SR003 reports the registry. The spec rules must not fail.
	s := move_spec([to_hook], {})
	not "SV113" in rules_of(dpl.deny) with input as s with data.project as {"services": "x"}
	count(dpl.warn) == 0 with input as s with data.project as {"services": "x"}
}
