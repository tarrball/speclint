package speclint.lib

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

# path_key returns the path of key in the mapping at prefix, for a finding.
# A key with a character other than a letter, a digit, "_", or "-" is in
# brackets and quotes, for example services["Order.Accessor"]. Thus speclint
# finds the line of the key. An empty prefix is the top level.
path_key(prefix, key) := $"{prefix}[{json.marshal(key)}]" if {
	not regex.match(`^[A-Za-z0-9_-]+$`, key)
} else := key if {
	prefix == ""
} else := $"{prefix}.{key}"

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
	not consonant_y(word)
}

# plural writes "utility" as "utilities", but "key" as "keys".
plural(word) := $"{trim_suffix(word, "y")}ies" if {
	is_string(word)
	consonant_y(word)
}

consonant_y(word) if regex.match(`[^aeiouAEIOU]y$`, word)

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
