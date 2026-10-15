package specval.spec.project

import data.specval.lib
import rego.v1

# METADATA
# title: Only OrderManager calls PricingEngine
# description: Pricing is orchestrated in one place so discounts are applied consistently.
# custom:
#   id: ORD001
deny contains f if {
	some i, c in input.calls
	c.to == "PricingEngine"
	c.from != "OrderManager"
	f := lib.finding(
		rego.metadata.rule(),
		sprintf("calls[%d]", [i]),
		sprintf("%s may not call PricingEngine", [c.from]),
		"Have OrderManager call PricingEngine and pass the price to whoever needs it.",
	)
}
