# The project rules of specval. They check the text of a spec, and the size
# and kind of the change. They do not use the layers or the registry.
package specval.spec.plan

import data.specval.lib
import rego.v1

limits := object.get(data.project, "plan", {})

max_calls := object.get(limits, "max_calls", 8)

max_acceptance := object.get(limits, "max_acceptance", 5)

max_words := object.get(data.project, ["ste", "max_words"], 25)

words := object.get(data.project, ["ste", "words"], {})

acceptance := object.get(input, "acceptance", [])

calls := object.get(input, "calls", [])

new_list := object.get(input, ["services", "new"], [])

out_of_scope := object.get(input, "out_of_scope", [])

# texts maps the path of each field of free text to its text.
texts["title"] := input.title

texts["summary"] := input.summary

texts[path] := ac.text if {
	some i, ac in acceptance
	path := $"acceptance[{i}].text"
}

texts[path] := text if {
	some i, text in out_of_scope
	path := $"out_of_scope[{i}]"
}

# word_count returns the number of words in s.
word_count(s) := count([w | some w in regex.split(`\s+`, trim_space(s)); w != ""])

# has_word is true if text has word, with no letter before or after it. The
# case of the letters is not important.
has_word(text, word) if {
	escaped := strings.replace_n({".": `\.`, "-": `\-`}, word)
	regex.match($`(?i)(^|[^a-z])({escaped})($|[^a-z])`, text)
}

# METADATA
# title: Each acceptance criterion is Given, when, then
# description: >-
#   The text of each acceptance criterion has the form "Given ..., when ..., then
#   ...". Thus each criterion is a test.
# custom:
#   id: PLN001
warn contains f if {
	some i, ac in acceptance
	not regex.match(`(?i)^given\b.+\bwhen\b.+\bthen\b`, ac.text)
	f := lib.finding(
		rego.metadata.rule(),
		$"acceptance[{i}].text",
		$"{ac.id} does not have the form Given ..., when ..., then ...",
		$"Write {ac.id} as: Given <a state>, when <an action>, then <a result that you can test>.",
	)
}

# METADATA
# title: A spec is small
# description: >-
#   A spec has 8 or fewer calls and 5 or fewer acceptance criteria
#   (.specval/data/plan.yaml). A larger change needs more than one pull request.
# custom:
#   id: PLN002
warn contains f if {
	some [field, items, limit] in [["calls", calls, max_calls], ["acceptance", acceptance, max_acceptance]]
	n := count(items)
	n > limit
	f := lib.finding(
		rego.metadata.rule(),
		field,
		$"The spec has {n} items in {field}, and the limit is {limit}",
		"Divide the change into two or more specs. Make one pull request for each spec.",
	)
}

# METADATA
# title: A bugfix or a chore does not add a service
# description: A new service is a feature. A bugfix or a chore changes the services that exist.
# custom:
#   id: PLN003
warn contains f if {
	input.kind in {"bugfix", "chore"}
	some i, s in new_list
	f := lib.finding(
		rego.metadata.rule(),
		$"services.new[{i}]",
		$"A {input.kind} adds the service {s.name}",
		"Change the kind to feature, or make the change in the services that exist.",
	)
}

# METADATA
# title: A feature tells what it does not do
# description: A feature has one or more items in `out_of_scope`.
# custom:
#   id: PLN004
warn contains f if {
	input.kind == "feature"
	count(out_of_scope) == 0
	f := lib.finding(
		rego.metadata.rule(),
		"out_of_scope",
		"The feature has no out_of_scope",
		"Add out_of_scope with one or more things that a reader can expect, but that this change does not do.",
	)
}

# METADATA
# title: Each sentence is short
# description: Each sentence in the text of a spec has 25 or fewer words (ASD-STE100).
# custom:
#   id: STE001
warn contains f if {
	some path, text in texts
	some sentence in regex.split(`[.!?](\s+|$)`, text)
	n := word_count(sentence)
	n > max_words
	f := lib.finding(
		rego.metadata.rule(),
		path,
		$"A sentence in {path} has {n} words, and the limit is {max_words}",
		$"Divide the sentence into two or more sentences of {max_words} words or fewer.",
	)
}

# METADATA
# title: The text uses approved words
# description: The text of a spec does not use the words in .specval/data/plan.yaml (ASD-STE100).
# custom:
#   id: STE002
warn contains f if {
	some path, text in texts
	some word, fix in words
	has_word(text, word)
	f := lib.finding(rego.metadata.rule(), path, $"{path} has the word {lib.quote(word)}", fix)
}

# METADATA
# title: The title has no period at the end
# custom:
#   id: STE003
warn contains f if {
	endswith(input.title, ".")
	f := lib.finding(
		rego.metadata.rule(),
		"title",
		"The title ends with a period",
		"Remove the period at the end of the title.",
	)
}
