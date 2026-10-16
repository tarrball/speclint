package specval.lib

import rego.v1

# finding makes a finding from the METADATA of the rule that calls it. Thus
# the rule ID and the title are in one place.
# Call it as lib.finding(rego.metadata.rule(), path, msg, fix).
finding(meta, path, msg, fix) := {
	"rule": object.get(meta, ["custom", "id"], "UNNAMED"),
	"title": object.get(meta, "title", ""),
	"path": path,
	"msg": msg,
	"fix": fix,
}

# join writes ["a", "b", "c"] as "a, b <word> c".
join(xs, _) := xs[0] if count(xs) == 1

join(xs, word) := $"{concat(", ", head)} {word} {last}" if {
	count(xs) > 1
	head := array.slice(xs, 0, count(xs) - 1)
	last := xs[count(xs) - 1]
}

# plural is undefined if word is not a string. Without this condition,
# interpolation would write the number 7 as "7s".
plural(word) := $"{word}s" if {
	is_string(word)
	not endswith(word, "y")
}

# plural writes "utility" as "utilities".
plural(word) := $"{trim_suffix(word, "y")}ies" if {
	is_string(word)
	endswith(word, "y")
}

# quote writes s as a quoted string with Go escapes, for example "got \"x\"".
# json.marshal is different: it escapes <, >, and & for HTML.
# regal ignore:prefer-string-interpolation
quote(s) := sprintf("%q", [s]) if is_string(s)

# quote writes a value that is not a string as JSON. sprintf("%q") would
# write the number 7 as the character '\a'.
quote(s) := json.marshal(s) if not is_string(s)

capitalize(s) := concat("", [upper(substring(s, 0, 1)), substring(s, 1, -1)])

# ordered returns the layers in xs in the layer order, from manager to resource.
ordered(xs) := [l | some l in data.dpl.layer_order; l in xs]
