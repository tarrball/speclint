# Project-specific spec rules. Each deny (error) or warn rule needs a METADATA
# block with a title and custom.id, and should build its result with
# lib.finding(rego.metadata.rule(), path, msg, fix). Example:
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
# 		$"{c.from} may not call PricingEngine",
# 		"Have OrderManager call PricingEngine and pass the result along.")
# }
package specval.spec.project

import data.specval.lib
import rego.v1
