package schema_test

import (
	"testing"

	"github.com/onereallylongname/avredit/internal/schema"
)

func TestValidateName(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"userId", true},
		{"_private", true},
		{"CamelCase", true},
		{"with_123", true},
		{"", false},
		{"  ", false},
		{"123start", false},
		{"has-dash", false},
		{"has space", false},
		{"has.dot", false},
	}

	for _, tt := range tests {
		result := schema.ValidateName(tt.input)
		if result.Valid != tt.valid {
			t.Errorf("ValidateName(%q) = %v, want %v (msg: %s)", tt.input, result.Valid, tt.valid, result.Message)
		}
	}
}

func TestValidateNamespace(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"", true},
		{"com.example", true},
		{"com.example.v1", true},
		{"single", true},
		{"com.123bad", false},
		{"com..double", false},
		{".leading", false},
	}

	for _, tt := range tests {
		result := schema.ValidateNamespace(tt.input)
		if result.Valid != tt.valid {
			t.Errorf("ValidateNamespace(%q) = %v, want %v (msg: %s)", tt.input, result.Valid, tt.valid, result.Message)
		}
	}
}

func TestValidateSymbol(t *testing.T) {
	existing := []string{"RED", "GREEN"}

	tests := []struct {
		input string
		valid bool
	}{
		{"BLUE", true},
		{"RED", false},  // duplicate
		{"123", false},  // invalid name
		{"", false},
	}

	for _, tt := range tests {
		result := schema.ValidateSymbol(tt.input, existing)
		if result.Valid != tt.valid {
			t.Errorf("ValidateSymbol(%q) = %v, want %v (msg: %s)", tt.input, result.Valid, tt.valid, result.Message)
		}
	}
}

func TestValidateFixedSize(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"16", true},
		{"1", true},
		{"0", false},
		{"-1", false},
		{"abc", false},
	}

	for _, tt := range tests {
		result := schema.ValidateFixedSize(tt.input)
		if result.Valid != tt.valid {
			t.Errorf("ValidateFixedSize(%q) = %v, want %v", tt.input, result.Valid, tt.valid)
		}
	}
}

func TestValidateDecimalPrecision(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"10", true},
		{"1", true},
		{"0", false},
		{"-5", false},
	}

	for _, tt := range tests {
		result := schema.ValidateDecimalPrecision(tt.input)
		if result.Valid != tt.valid {
			t.Errorf("ValidateDecimalPrecision(%q) = %v, want %v", tt.input, result.Valid, tt.valid)
		}
	}
}

func TestValidateDecimalScale(t *testing.T) {
	tests := []struct {
		scale     string
		precision string
		valid     bool
	}{
		{"2", "10", true},
		{"0", "5", true},
		{"10", "10", true},
		{"11", "10", false},  // scale > precision
		{"-1", "10", false},
	}

	for _, tt := range tests {
		result := schema.ValidateDecimalScale(tt.scale, tt.precision)
		if result.Valid != tt.valid {
			t.Errorf("ValidateDecimalScale(%q, %q) = %v, want %v (msg: %s)", tt.scale, tt.precision, result.Valid, tt.valid, result.Message)
		}
	}
}

func TestValidateUnionBranchAdd(t *testing.T) {
	tests := []struct {
		newType  string
		existing []string
		valid    bool
	}{
		{"string", []string{"null"}, true},
		{"string", []string{"null", "string"}, false},  // duplicate
		{"union", []string{"null"}, false},             // nested union
		{"record", []string{"null", "record"}, true},   // named types can repeat
	}

	for _, tt := range tests {
		result := schema.ValidateUnionBranchAdd(tt.newType, tt.existing)
		if result.Valid != tt.valid {
			t.Errorf("ValidateUnionBranchAdd(%q, %v) = %v, want %v (msg: %s)", tt.newType, tt.existing, result.Valid, tt.valid, result.Message)
		}
	}
}

func TestIsPrimitive(t *testing.T) {
	if !schema.IsPrimitive("int") {
		t.Error("int should be primitive")
	}
	if schema.IsPrimitive("record") {
		t.Error("record should not be primitive")
	}
}

func TestSlotAcceptsKind(t *testing.T) {
	if !schema.SlotAcceptsKind(schema.SlotRecordFields, schema.KindField) {
		t.Error("record.fields should accept field")
	}
	if schema.SlotAcceptsKind(schema.SlotRecordFields, schema.KindPrimitive) {
		t.Error("record.fields should not accept primitive")
	}
	if !schema.SlotAcceptsKind(schema.SlotUnionBranch, schema.KindPrimitive) {
		t.Error("union.branch should accept primitive")
	}
}
