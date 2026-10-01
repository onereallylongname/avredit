package search_test

import (
	"encoding/json"
	"testing"

	"github.com/onereallylongname/avredit/internal/projection"
	"github.com/onereallylongname/avredit/internal/search"
)

func mustBuild(t *testing.T, jsonStr string) *projection.Projection {
	t.Helper()
	var raw map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		t.Fatalf("invalid test JSON: %v", err)
	}
	proj, err := projection.Build(raw)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	return proj
}

const searchTestSchema = `{
	"type": "record",
	"name": "UserEvent",
	"namespace": "com.analytics",
	"fields": [
		{"name": "eventId", "type": "string"},
		{"name": "userId", "type": ["null", "string"]},
		{"name": "timestamp", "type": {"type": "long", "logicalType": "timestamp-millis"}},
		{"name": "metadata", "type": {"type": "map", "values": "string"}},
		{"name": "region", "type": "string", "doc": "The AWS region where the event was captured, e.g. us-east-1."}
	]
}`

// --- Fuzzy Tests ---

func TestFuzzy_Subsequence(t *testing.T) {
	tests := []struct {
		str, pattern string
		want         bool
	}{
		{"UserEvent", "UE", true},   // CamelCase
		{"eventId", "eid", true},    // subsequence
		{"userId", "xyz", false},    // no match
		{"timestamp", "time", true}, // prefix-ish
		{"", "x", false},            // empty string
		{"abc", "", false},          // empty pattern
	}

	for _, tt := range tests {
		got := search.Fuzzy(tt.str, tt.pattern)
		if got != tt.want {
			t.Errorf("Fuzzy(%q, %q) = %v, want %v", tt.str, tt.pattern, got, tt.want)
		}
	}
}

func TestFuzzyScore(t *testing.T) {
	// Exact match should score 1.0
	if score := search.FuzzyScore("eventId", "eventId"); score != 1.0 {
		t.Errorf("exact match score = %f, want 1.0", score)
	}

	// Prefix should score 0.9
	if score := search.FuzzyScore("eventId", "event"); score != 0.9 {
		t.Errorf("prefix score = %f, want 0.9", score)
	}

	// No match should score 0
	if score := search.FuzzyScore("eventId", "xyz"); score != 0 {
		t.Errorf("no-match score = %f, want 0", score)
	}
}

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "abc", 0},
		{"abc", "ab", 1},
		{"kitten", "sitting", 3},
	}

	for _, tt := range tests {
		got := search.Levenshtein(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("Levenshtein(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestTypeComponentScore(t *testing.T) {
	// Exact component match
	if score := search.TypeComponentScore("union,null,string", "string"); score != 1.0 {
		t.Errorf("exact component = %f, want 1.0", score)
	}

	// Prefix component match
	if score := search.TypeComponentScore("timestamp-millis", "time"); score != 0.9 {
		t.Errorf("prefix component = %f, want 0.9", score)
	}

	// No match
	if score := search.TypeComponentScore("int", "string"); score != 0 {
		t.Errorf("no match = %f, want 0", score)
	}
}

func TestDocScore(t *testing.T) {
	doc := "The AWS region where the event was captured, e.g. us-east-1."

	if score := search.DocScore(doc, "region"); score <= 0 {
		t.Errorf("whole-word substring match score = %f, want > 0", score)
	}
	if score := search.DocScore(doc, "regio"); score <= 0 {
		t.Errorf("partial substring match score = %f, want > 0", score)
	}
	if score := search.DocScore(doc, "nonexistent"); score != 0 {
		t.Errorf("no match score = %f, want 0", score)
	}
	if score := search.DocScore("", "region"); score != 0 {
		t.Errorf("empty doc score = %f, want 0", score)
	}
	if score := search.DocScore(doc, ""); score != 0 {
		t.Errorf("empty needle score = %f, want 0", score)
	}
}

// --- ParseQuery Tests ---

func TestParseQuery(t *testing.T) {
	tests := []struct {
		input string
		want  search.Filters
	}{
		{"n:eventId", search.Filters{Name: "eventId"}},
		{"t:string", search.Filters{Type: "string"}},
		{"p:UserEvent", search.Filters{Parent: "UserEvent"}},
		{"ns:com.analytics", search.Filters{Namespace: "com.analytics"}},
		{"a:Alias1", search.Filters{Alias: "Alias1"}},
		{"d:deprecated", search.Filters{Doc: "deprecated"}},
		{"userId", search.Filters{Text: "userId"}},
		{"", search.Filters{}},
	}

	for _, tt := range tests {
		got := search.ParseQuery(tt.input)
		if got != tt.want {
			t.Errorf("ParseQuery(%q) = %+v, want %+v", tt.input, got, tt.want)
		}
	}
}

// --- QueryNodes Tests ---

func TestQueryNodes_ByName(t *testing.T) {
	proj := mustBuild(t, searchTestSchema)
	results := search.QueryNodes(proj, search.Filters{Name: "eventId"})

	if len(results) == 0 {
		t.Fatal("expected results for name search 'eventId'")
	}

	// First result should be the eventId field
	if results[0].Node.Name() != "eventId" {
		t.Errorf("first result name = %q, want 'eventId'", results[0].Node.Name())
	}
}

func TestQueryNodes_ByType(t *testing.T) {
	proj := mustBuild(t, searchTestSchema)
	results := search.QueryNodes(proj, search.Filters{Type: "string"})

	if len(results) == 0 {
		t.Fatal("expected results for type search 'string'")
	}
}

func TestQueryNodes_FreeText(t *testing.T) {
	proj := mustBuild(t, searchTestSchema)
	results := search.QueryNodes(proj, search.Filters{Text: "user"})

	if len(results) == 0 {
		t.Fatal("expected results for free text 'user'")
	}
}

func TestQueryNodes_ByDoc(t *testing.T) {
	proj := mustBuild(t, searchTestSchema)
	results := search.QueryNodes(proj, search.Filters{Doc: "aws region"})

	if len(results) == 0 {
		t.Fatal("expected results for doc search 'aws region'")
	}
	found := false
	for _, r := range results {
		if r.Node.Name() == "region" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'region' field to match doc search, got %+v", results)
	}
}

func TestQueryNodes_FreeTextMatchesDoc(t *testing.T) {
	proj := mustBuild(t, searchTestSchema)
	// "us-east-1" only appears in the doc text, not in any name/type/alias.
	results := search.QueryNodes(proj, search.Filters{Text: "us-east-1"})

	if len(results) == 0 {
		t.Fatal("expected free-text search to find matches inside doc field content")
	}
	found := false
	for _, r := range results {
		if r.Node.Name() == "region" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'region' field to be found via doc-text free search, got %+v", results)
	}
}

func TestQueryNodes_Empty(t *testing.T) {
	proj := mustBuild(t, searchTestSchema)
	results := search.QueryNodes(proj, search.Filters{})

	if len(results) != 0 {
		t.Errorf("empty filter should return no results, got %d", len(results))
	}
}
