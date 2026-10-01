// Package validation provides advisory schema analysis for avredit projections.
//
// Design principles:
//   - Pure analysis: never blocks mutations, only reports findings
//   - Zero TUI dependency: operates solely on projection.Projection
//   - Module-ready: exposes Analyze(proj) → []Issue, trivially wrappable in a future Module
//   - Incremental: rules are independent functions, easy to add/remove/toggle
//
// Integration:
//   - Called AFTER mutations to report schema issues
//   - Results displayed as notifications/warnings in the UI layer
//   - Future: ValidationModule wraps this package in the Module interface
//
// Severity model:
//   - Error: structural violation (invalid Avro, will fail serialization)
//   - Warning: best-practice violation (duplicate names, missing defaults)
//   - Info: advisory (documentation suggestions)
package validation

import (
	"github.com/onereallylongname/avredit/internal/projection"
)

// Severity classifies the importance of a validation issue.
type Severity int

const (
	SeverityError   Severity = iota // Schema will not serialize correctly
	SeverityWarning Severity = iota // Valid but problematic (duplicates, missing defaults)
	SeverityInfo    Severity = iota // Advisory (documentation, best practices)
)

// String returns the display name of the severity.
func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "ERROR"
	case SeverityWarning:
		return "WARNING"
	case SeverityInfo:
		return "INFO"
	default:
		return "UNKNOWN"
	}
}

// Issue represents a single validation finding.
type Issue struct {
	Severity Severity
	NodeID   string // projection node where the issue was found
	Rule     string // machine-readable rule identifier (e.g., "duplicate-field-name")
	Message  string // human-readable description
}

// RuleFunc is the signature for individual validation rules.
// Each rule inspects the projection and returns zero or more issues.
type RuleFunc func(proj *projection.Projection) []Issue

// Analyzer holds the set of active validation rules.
// Designed for future toggling (per rule enable/disable via config or command).
type Analyzer struct {
	rules []namedRule
}

type namedRule struct {
	id   string
	fn   RuleFunc
	enabled bool
}

// NewAnalyzer creates an Analyzer with the default rule set.
func NewAnalyzer() *Analyzer {
	a := &Analyzer{}
	a.rules = []namedRule{
		{id: "duplicate-field-name", fn: RuleDuplicateFieldNames, enabled: true},
		{id: "circular-reference", fn: RuleCircularReferences, enabled: true},
		{id: "empty-record", fn: RuleEmptyRecords, enabled: true},
		{id: "empty-union", fn: RuleEmptyUnions, enabled: true},
	}
	return a
}

// Analyze runs all enabled rules against the projection and returns all issues.
// Safe to call with nil projection (returns empty).
func (a *Analyzer) Analyze(proj *projection.Projection) []Issue {
	if proj == nil || proj.RootID == "" {
		return nil
	}
	var issues []Issue
	for _, r := range a.rules {
		if r.enabled {
			issues = append(issues, r.fn(proj)...)
		}
	}
	return issues
}

// SetEnabled enables or disables a rule by ID.
func (a *Analyzer) SetEnabled(ruleID string, enabled bool) {
	for i := range a.rules {
		if a.rules[i].id == ruleID {
			a.rules[i].enabled = enabled
			return
		}
	}
}

// RuleIDs returns the list of registered rule identifiers.
func (a *Analyzer) RuleIDs() []string {
	ids := make([]string, len(a.rules))
	for i, r := range a.rules {
		ids[i] = r.id
	}
	return ids
}

// ErrorCount returns the number of issues with Error severity.
func ErrorCount(issues []Issue) int {
	n := 0
	for _, iss := range issues {
		if iss.Severity == SeverityError {
			n++
		}
	}
	return n
}

// WarningCount returns the number of issues with Warning severity.
func WarningCount(issues []Issue) int {
	n := 0
	for _, iss := range issues {
		if iss.Severity == SeverityWarning {
			n++
		}
	}
	return n
}

// IssuesForNode filters issues to those affecting a specific node.
func IssuesForNode(issues []Issue, nodeID string) []Issue {
	var filtered []Issue
	for _, iss := range issues {
		if iss.NodeID == nodeID {
			filtered = append(filtered, iss)
		}
	}
	return filtered
}
