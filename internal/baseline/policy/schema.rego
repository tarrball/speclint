package specval.spec.schema

import data.specval.lib
import rego.v1

# Generated from schema.json at load time: allowed keys and descriptions per
# path, with array indexes written as "*" (e.g. "calls.*.op").
keys := data.dpl.schema_keys

hints := data.dpl.schema_hints

# METADATA
# title: Spec matches the specval format
# description: >-
#   Required fields, field names, and value types follow the schema (`specval schema`).
#   Other rules run only once this passes.
# custom:
#   id: SV001
deny contains f if {
	[ok, errs] := json.match_schema(input, data.dpl.schema)
	not ok
	some e in errs
	f := lib.finding(rego.metadata.rule(), display(e.field), e.desc, fix(e))
}

is_root(field) if lower(field) == "(root)"

# "calls.2.to" -> "calls[2].to"
display(field) := "" if is_root(field)

display(field) := regex.replace(field, `\.(\d+)`, "[$1]") if not is_root(field)

# "calls.2.to" -> "calls.*.to"
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
	msg := sprintf("Remove `%s` or fix its name. Allowed here: %s.", [p, concat(", ", keys[norm(e.field)])])
} else := msg if {
	msg := hints[norm(e.field)]
} else := "See `specval schema` for the expected format."
