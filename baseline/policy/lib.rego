package specval.lib

import rego.v1

# finding builds a result from the calling rule's METADATA, so the rule id and
# title live in one place. Call it as lib.finding(rego.metadata.rule(), ...).
finding(meta, path, msg, fix) := {
	"rule": object.get(meta, ["custom", "id"], "UNNAMED"),
	"title": object.get(meta, "title", ""),
	"path": path,
	"msg": msg,
	"fix": fix,
}

# join renders ["a", "b", "c"] as "a, b <word> c".
join(xs, _) := xs[0] if count(xs) == 1

join(xs, word) := $"{concat(", ", head)} {word} {last}" if {
	count(xs) > 1
	head := array.slice(xs, 0, count(xs) - 1)
	last := xs[count(xs) - 1]
}

# plural is undefined if word is not a string, as concat was. Interpolation
# would write a number such as 7 as "7s".
plural(word) := $"{word}s" if is_string(word)

# quote renders s as a quoted string with Go escapes, like "got \"x\"".
# json.marshal is different: it escapes <, >, and & for HTML.
# regal ignore:prefer-string-interpolation
quote(s) := sprintf("%q", [s])

capitalize(s) := concat("", [upper(substring(s, 0, 1)), substring(s, 1, -1)])

# ordered returns the layers in xs in architectural order (manager first).
ordered(xs) := [l | some l in data.dpl.layer_order; l in xs]
