package specval.spec.schema_test

import data.specval.spec.schema
import rego.v1

valid := {
	"specval": 1,
	"id": "A-1",
	"kind": "feature",
	"title": "T",
	"summary": "S",
	"acceptance": [{"id": "AC1", "text": "T"}],
	"calls": [],
}

test_a_valid_spec_has_no_findings if {
	count(schema.deny) == 0 with input as valid
}

test_display if {
	schema.display("(root)") == ""
	schema.display("(Root)") == ""
	schema.display("calls.2.to") == "calls[2].to"
	schema.display("calls.12.covers.3") == "calls[12].covers[3]"
}

test_norm if {
	schema.norm("(root)") == "(root)"
	schema.norm("calls.2.to") == "calls.*.to"
}

test_child if {
	schema.child("(root)", "id") == "id"
	schema.child("calls.2", "op") == "calls.*.op"
}

test_missing_field_fix_uses_the_hint if {
	some f in schema.deny with input as object.remove(valid, ["kind"])
	f.rule == "SV001"
	f.path == ""
	f.fix == "Add `kind`: The kind of change: feature, bugfix, or chore."
}

test_missing_nested_field if {
	spec := object.union(valid, {"calls": [{"from": "A", "to": "B", "op": "Get"}]})
	some f in schema.deny with input as spec
	f.path == "calls[0]"
	startswith(f.fix, "Add `covers`: ")
}

test_unknown_field_fix_lists_the_allowed_fields if {
	spec := object.union(valid, {"services": {"removed": []}})
	some f in schema.deny with input as spec
	f.path == "services"
	f.fix == "Remove `removed`, or correct its name. The permitted fields here are: changed, new, relayered."
}

test_wrong_value_fix_uses_the_hint if {
	spec := object.union(valid, {"acceptance": [{"id": "A1", "text": "T"}]})
	some f in schema.deny with input as spec
	f.path == "acceptance[0].id"
	f.fix == "Correct the value of `acceptance[0].id`. AC and a number, for example AC1."
}

test_wrong_type_fix_gives_the_shape if {
	# The fix tells the correct form from the schema, so that an agent does
	# not need `sv schema`.
	spec := object.union(valid, {
		"services": ["CheckManager"],
		"acceptance": ["The output does not change."],
		"out_of_scope": [1],
	})
	fixes := {f.path: f.fix | some f in schema.deny with input as spec}
	fixes.services == "Write `services` as an object with the fields changed, new, relayered."
	fixes["acceptance[0]"] == "Write `acceptance[0]` as an object with the fields id, text (required: id, text)."
	fixes["out_of_scope[0]"] == "Write `out_of_scope[0]` as a text value."
}

test_wrong_type_fix_for_a_list_has_the_hint if {
	spec := object.union(valid, {"calls": [{"from": "A", "to": "B", "op": "X", "covers": "AC1"}]})
	some f in schema.deny with input as spec
	f.path == "calls[0].covers"
	startswith(f.fix, "Write `calls[0].covers` as a list. Each item is a text value. The acceptance criteria")
}
