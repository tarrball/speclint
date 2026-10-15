package policy

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strconv"
)

// Finding is one problem that a rule found.
type Finding struct {
	Rule     string `json:"rule"`
	Title    string `json:"title,omitempty"`
	Severity string `json:"severity"`
	Path     string `json:"path"`
	Line     int    `json:"line,omitempty"`
	Msg      string `json:"msg"`
	Fix      string `json:"fix"`
}

// RuleDoc describes one rule, from its METADATA annotation.
type RuleDoc struct {
	ID, Title, Description, Severity, Package string
}

func decodeFindings(v any, severity string) ([]Finding, error) {
	var found []Finding
	if v == nil {
		return nil, nil
	}
	if err := roundTrip(v, &found); err != nil {
		return nil, fmt.Errorf("rule returned a malformed finding: %w", err)
	}
	for i := range found {
		found[i].Severity = severity
	}
	return found, nil
}

func sortFindings(found []Finding) {
	slices.SortStableFunc(found, func(a, b Finding) int {
		return cmp.Or(cmp.Compare(a.Line, b.Line), cmp.Compare(a.Rule, b.Rule))
	})
}

var yamlLineRe = regexp.MustCompile(`line (\d+)`)

func yamlError(err error) Finding {
	f := Finding{Rule: "SV000", Title: "Spec is valid YAML", Severity: "error", Msg: err.Error(), Fix: "Fix the YAML syntax at this line."}
	if m := yamlLineRe.FindStringSubmatch(err.Error()); m != nil {
		f.Line, _ = strconv.Atoi(m[1])
	}
	return f
}
