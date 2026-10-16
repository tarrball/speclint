package specval.spec.schema

import data.specval.lib
import rego.v1

# specval makes keys and hints from schema.json when it loads the rules. keys
# gives the permitted keys at each path, and hints gives the description of
# each path. In a path, "*" is an array index, for example "calls.*.op".
keys := data.dpl.schema_keys

hints := data.dpl.schema_hints

# types gives the type of each path, and required gives the required keys of
# each object, for the fix of a value that has the wrong type.
types := data.dpl.schema_types

required := data.dpl.schema_required

# METADATA
# title: The spec matches the schema
# description: >-
#   The spec has the fields, the field names, and the value types that the schema
#   (`sv schema`) gives. The other rules run only after this rule passes.
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
	e.type == "invalid_type"
	want := shape(norm(e.field))
	msg := with_hint($"Write {name(e.field)} as {want}.", norm(e.field))
} else := msg if {
	hint := hints[norm(e.field)]
	msg := $"Correct the value of `{display(e.field)}`. {hint}"
} else := "Run `sv schema` to see the correct format."

# name writes a path for a fix: the spec itself, or the path in backticks.
name(field) := "the spec" if is_root(field)

name(field) := $"`{display(field)}`" if not is_root(field)

# shape tells the correct form of the value at a normalized path, from the
# schema. For example: "a list. Each item is an object with the fields id, text".
shape(path) := $"a list. Each item is {item}" if {
	types[path] == "array"
	item := simple_shape(child(path, "*"))
}

shape(path) := "a list" if {
	types[path] == "array"
	not simple_shape(child(path, "*"))
}

shape(path) := simple_shape(path) if types[path] != "array"

# simple_shape tells the form of a value that is not a list. Rego does not
# permit a recursive rule, so a list in a list is only "a list".
simple_shape(path) := $"an object with the fields {fields(path)}" if types[path] == "object"

simple_shape(path) := "a text value" if types[path] == "string"

simple_shape(path) := "a number" if types[path] in {"integer", "number"}

simple_shape(path) := "true or false" if types[path] == "boolean"

# fields lists the keys of the object at path, and tells which are required.
fields(path) := $"{concat(", ", keys[path])} (required: {concat(", ", req)})" if {
	req := required[path]
	count(req) > 0
} else := concat(", ", keys[path])

# with_hint adds the description of path to text, if the schema has one.
with_hint(text, path) := $"{text} {hint}" if {
	hint := hints[path]
} else := text
