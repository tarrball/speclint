package specval.spec.schema

import data.specval.lib
import rego.v1

# specval makes keys and hints from schema.json when it loads the rules. keys
# gives the permitted keys at each path, and hints gives the description of
# each path. In a path, "*" is an array index, for example "calls.*.op".
keys := data.dpl.schema_keys

hints := data.dpl.schema_hints

# METADATA
# title: The spec matches the schema
# description: >-
#   The spec has the fields, the field names, and the value types that the schema
#   (`specval schema`) gives. The other rules run only after this rule passes.
# custom:
#   id: SV001
deny contains f if {
	[ok, errs] := json.match_schema(input, data.dpl.schema)
	not ok
	some e in errs
	f := lib.finding(rego.metadata.rule(), display(e.field), e.desc, fix(e))
}

is_root(field) if lower(field) == "(root)"

# display writes the path "calls.2.to" as "calls[2].to".
display(field) := "" if is_root(field)

display(field) := regex.replace(field, `\.(\d+)`, "[$1]") if not is_root(field)

# norm writes the path "calls.2.to" as "calls.*.to".
norm(field) := "(root)" if is_root(field)

norm(field) := regex.replace(field, `\.\d+`, ".*") if not is_root(field)

child(field, prop) := prop if is_root(field)

child(field, prop) := concat(".", [norm(field), prop]) if not is_root(field)

fix(e) := msg if {
	e.type == "required"
	p := split(e.desc, " ")[0]
	msg := sprintf("Add `%s`: %s", [p, hints[child(e.field, p)]])
} else := msg if {
	e.type == "required"
	msg := sprintf("Add `%s`.", [split(e.desc, " ")[0]])
} else := msg if {
	e.type == "additional_property_not_allowed"
	p := split(e.desc, " ")[2]
	permitted := concat(", ", keys[norm(e.field)])
	msg := $"Remove `{p}`, or correct its name. The permitted fields here are: {permitted}."
} else := msg if {
	hint := hints[norm(e.field)]
	msg := $"Correct the value of `{display(e.field)}`. {hint}"
} else := "Run `specval schema` to see the correct format."
