package specval.lib_test

import data.specval.lib
import rego.v1

test_finding_reads_the_rule_metadata if {
	meta := {"title": "A title", "custom": {"id": "X001"}}
	lib.finding(meta, "calls[0]", "msg", "fix") == {
		"rule": "X001",
		"title": "A title",
		"path": "calls[0]",
		"msg": "msg",
		"fix": "fix",
	}
}

test_finding_without_metadata if {
	f := lib.finding({}, "", "msg", "fix")
	f.rule == "UNNAMED"
	f.title == ""
}

test_join if {
	lib.join(["a"], "and") == "a"
	lib.join(["a", "b"], "and") == "a and b"
	lib.join(["a", "b", "c"], "or") == "a, b or c"
}

test_join_of_nothing_is_undefined if {
	not lib.join([], "and")
}

test_plural if {
	lib.plural("engine") == "engines"
}

test_plural_of_a_number_is_undefined if {
	not lib.plural(7)
}

test_capitalize if {
	lib.capitalize("accessors") == "Accessors"
	lib.capitalize("x") == "X"
}

test_ordered_follows_the_layer_order if {
	lib.ordered(["resource", "manager", "engine"]) == ["manager", "engine", "resource"]
	lib.ordered(["nope", "accessor"]) == ["accessor"]
}

test_quote_uses_go_escapes if {
	lib.quote("storage") == `"storage"`
	lib.quote("") == `""`
	lib.quote("<b>&") == `"<b>&"`
}
