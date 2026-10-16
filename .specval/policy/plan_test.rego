package specval.spec.plan_test

import data.specval.spec.plan
import rego.v1

project := {
	"plan": {"max_calls": 2, "max_acceptance": 1},
	"ste": {"max_words": 12, "words": {"should": "Use must.", "etc.": "Write each item."}},
}

ac1 := {"id": "AC1", "text": "Given a spec, when Claude saves it, then the hook checks it."}

call := {"from": "HookCommand", "to": "PolicyEngine", "op": "CheckSpec", "covers": ["AC1"]}

# spec returns a spec that passes each rule, with the fields in changes replaced.
spec(changes) := object.union(
	{
		"specval": 1,
		"id": "A-1",
		"kind": "feature",
		"title": "Check a spec",
		"summary": "The hook checks a spec.",
		"acceptance": [ac1],
		"calls": [call],
		"out_of_scope": ["Code files"],
	},
	changes,
)

test_a_good_spec_has_no_findings if {
	count(plan.warn) == 0 with input as spec({}) with data.project as project
}

test_pln001_given_when_then if {
	s := spec({"acceptance": [{"id": "AC1", "text": "The hook checks the spec."}]})
	found := {[f.rule, f.path] | some f in plan.warn with input as s with data.project as project}
	found == {["PLN001", "acceptance[0].text"]}
}

test_pln002_size if {
	ac2 := object.union(ac1, {"id": "AC2"})
	s := spec({"calls": [call, call, call], "acceptance": [ac1, ac2]})
	found := {[f.rule, f.path] | some f in plan.warn with input as s with data.project as project}
	found == {["PLN002", "calls"], ["PLN002", "acceptance"]}
}

test_pln003_a_bugfix_adds_a_service if {
	manager := {"name": "CheckManager", "layer": "manager", "purpose": "P"}
	s := spec({"kind": "bugfix", "services": {"new": [manager]}})
	found := {[f.rule, f.path] | some f in plan.warn with input as s with data.project as project}
	found == {["PLN003", "services.new[0]"]}
}

test_pln004_a_feature_without_out_of_scope if {
	feature := spec({"out_of_scope": []})
	found := {[f.rule, f.path] | some f in plan.warn with input as feature with data.project as project}
	found == {["PLN004", "out_of_scope"]}

	chore := spec({"kind": "chore", "out_of_scope": []})
	count(plan.warn) == 0 with input as chore with data.project as project
}

test_ste001_a_long_sentence if {
	long := "The hook checks each spec that Claude saves in the specs directory of the project."
	s := spec({"summary": $"The hook checks a spec. {long}"})
	found := {[f.rule, f.path] | some f in plan.warn with input as s with data.project as project}
	found == {["STE001", "summary"]}
}

test_ste002_words if {
	s := spec({
		"title": "Hook should check",
		"out_of_scope": ["Code, docs, etc."],
		"summary": "The hook checks a spec. It is a shoulder.",
	})
	found := {[f.rule, f.path] | some f in plan.warn with input as s with data.project as project}
	found == {["STE002", "title"], ["STE002", "out_of_scope[0]"]}
}

test_ste003_a_title_with_a_period if {
	s := spec({"title": "Check a spec."})
	found := {[f.rule, f.path] | some f in plan.warn with input as s with data.project as project}
	found == {["STE003", "title"]}
}

test_default_limits_without_data if {
	# Without project data, the rules use the limits in plan.rego.
	count(plan.warn) == 0 with input as spec({}) with data.project as {}
}
