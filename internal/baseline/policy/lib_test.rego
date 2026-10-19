package speclint.lib_test

import data.speclint.lib
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
	lib.plural("utility") == "utilities"
	lib.plural("key") == "keys"
	lib.plural("gateway") == "gateways"
	lib.plural("y") == "ys"
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

test_quote_writes_other_values_as_json if {
	lib.quote(7) == "7"
	lib.quote(true) == "true"
	lib.quote({"a": 1}) == `{"a":1}`
}

test_path_key if {
	lib.path_key("services", "OrderManager") == "services.OrderManager"
	lib.path_key("services", "x-y_1") == "services.x-y_1"
	lib.path_key("services", "Order.Accessor") == `services["Order.Accessor"]`
	lib.path_key("services", `a "b"`) == `services["a \"b\""]`
	lib.path_key("", "top") == "top"
	lib.path_key("", "a.b") == `["a.b"]`
}
