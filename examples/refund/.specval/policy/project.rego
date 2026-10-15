package specval.spec.project

import data.specval.lib
import rego.v1

# METADATA
# title: Only OrderManager calls PricingEngine
# description: One service controls pricing, so that all discounts apply in the same way.
# custom:
#   id: ORD001
deny contains f if {
	some i, c in input.calls
	c.to == "PricingEngine"
	c.from != "OrderManager"
	f := lib.finding(
		rego.metadata.rule(),
		$"calls[{i}]",
		$"{c.from} cannot call PricingEngine",
		"Make OrderManager call PricingEngine, and give the price to the services that need it.",
	)
}
