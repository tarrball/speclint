# The spec rules of this project. Each deny (error) or warn rule needs a
# block of METADATA with a title and a custom.id. Make each finding with
# lib.finding(rego.metadata.rule(), path, msg, fix). For example:
#
# # METADATA
# # title: Only OrderManager calls PricingEngine
# # custom:
# #   id: PRJ001
# deny contains f if {
# 	some i, c in input.calls
# 	c.to == "PricingEngine"
# 	c.from != "OrderManager"
# 	f := lib.finding(rego.metadata.rule(), $"calls[{i}]",
# 		$"{c.from} cannot call PricingEngine",
# 		"Make OrderManager call PricingEngine, and give the price to the services that need it.")
# }
package specval.spec.project

import data.specval.lib
import rego.v1
