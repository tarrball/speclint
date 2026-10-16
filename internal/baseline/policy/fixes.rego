package specval.spec.dpl

# This file holds the helpers that write the fix texts of the rules in dpl.rego.

import data.specval.lib
import rego.v1

can_call(l) := $"{lib.capitalize(lib.plural(l))} can call only {targets}" if {
	count(layers[l]) > 0
	targets := lib.join([lib.plural(t) | some t in lib.ordered(layers[l])], "and")
}

can_call(l) := $"{lib.capitalize(lib.plural(l))} cannot call other services" if count(layers[l]) == 0

# layer_fix is undefined if the layer of the caller is not known (SV105).
# String interpolation writes an undefined value as "<undefined>", so the
# rules bind each value that can be undefined before they use it.
layer_fix(from, to, target) := msg if {
	rule := can_call(from)
	callers := lib.ordered([l | some l, targets in layers; to in targets])
	count(callers) > 0
	msg := $"{rule}. Send the call to {target} through a {lib.join(callers, "or")}."
} else := msg if {
	rule := can_call(from)
	callee := lib.plural(to)
	msg := $"{rule}. No service can call {callee}."
}

# unknown_fix tells how to fix a reference to a service that is not registered.
# It is undefined if the registry is not an object. SR003 reports that case.
unknown_fix(name, action) := concat(" ", [
	$"If {name} is new, {action} services.new with a layer and purpose.",
	$"Otherwise use a registered service: {names}.",
]) if {
	# Bind the list first. If the registry is not an object, suggestions is
	# undefined, and so is this fix (SR003 reports the registry).
	list := suggestions(name)
	names := concat(", ", [label | some n in list; label := with_layer(n)])
}

# with_layer writes a registered service with its layer, for example
# "OrderManager (manager)", so that an agent can choose the correct service.
with_layer(name) := $"{name} ({layer})" if {
	layer := registry[name].layer
	is_string(layer)
} else := name

# call_problem tells why the layer rules or the limits refuse a call.
call_problem(from, to) := $"{lib.plural(fl)} cannot call {lib.plural(tl)}" if {
	fl := layer_of(from)
	tl := layer_of(to)
	not tl in object.get(layers, fl, [])
} else := "a limit in the registry refuses it"

# limit_fix tells how to fix a call that a limit in the registry refuses.
limit_fix(subject, verb, list, key, owner) := msg if {
	count(list) > 0
	names := lib.join(list, "or")
	msg := $"{subject} {verb} {names}. Change the call, or ask the owners of the registry to change {key} of {owner}."
}

limit_fix(subject, _, list, key, owner) := msg if {
	count(list) == 0
	msg := $"{subject} has an empty {key}. Remove the call, or ask the owners of the registry to change {key} of {owner}."
}

# move_work tells where the work of a removed call can go: the services that
# the caller can call. If the caller has may_call, only the items in may_call
# that the layers also permit are possible.
move_work(name) := msg if {
	allowed := registry[name].may_call
	is_array(allowed)
	msg := limited_move(name, allowed)
} else := msg if {
	callable := lib.ordered(object.get(layers, layer_of(name), []))
	count(callable) > 0
	msg := targets_move(name, callable)
} else := cannot_move(name)

limited_move(name, allowed) := msg if {
	callable := object.get(layers, layer_of(name), [])
	names := sort([t | some t in allowed; not layers[t]; layer_of(t) in callable])
	kinds := lib.ordered({t | some t in allowed; t in callable})
	targets := array.concat(names, kinds)
	count(targets) > 0
	msg := targets_move(name, targets)
} else := cannot_move(name)

targets_move(name, targets) := $"Then move the work to a service that {name} can call: {list}." if {
	list := lib.join([w | some t in targets; w := target_word(t)], "or")
}

# target_word writes a layer as a plural, for example "managers", and a
# service as its name.
target_word(t) := lib.plural(t) if layers[t]

target_word(t) := t if not layers[t]

cannot_move(name) := $"{name} cannot call other services, so move the work out of {name}."
