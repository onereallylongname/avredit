package projection

import (
	"fmt"

	"github.com/onereallylongname/avredit/internal/schema"
)

// Normalized holds the result of normalizing a raw Avro type value.
type Normalized struct {
	Kind     schema.NodeKind
	Type     string   // for primitives/named: the type string
	Fields   []any    // for records: the fields slice
	Items    any      // for arrays: the items type
	Values   any      // for maps: the values type
	Branches []any    // for unions: the branch types
	Raw      any      // original value
}

// NormalizeType inspects a raw Avro type value and returns its structural classification.
// This is the critical routing function that maps JSON values to node kinds:
//   - Plain string matching a primitive → KindPrimitive ("int", "string", etc.)
//   - Plain string not matching → KindNamed (reference like "Money", "com.acme.Order")
//   - JSON array → KindUnion (["null", "string"])
//   - JSON object with "type" field → dispatches to record/array/map/enum/fixed/primitive
func NormalizeType(typeVal any) (Normalized, error) {
	switch v := typeVal.(type) {
	case string:
		// Primitive or named reference
		if schema.IsPrimitive(v) {
			return Normalized{Kind: schema.KindPrimitive, Type: v, Raw: typeVal}, nil
		}
		return Normalized{Kind: schema.KindNamed, Type: v, Raw: typeVal}, nil

	case []any:
		// Union
		return Normalized{Kind: schema.KindUnion, Branches: v, Raw: typeVal}, nil

	case map[string]any:
		// Complex or logical type
		typeName, _ := v["type"].(string)
		if typeName == "" {
			return Normalized{}, fmt.Errorf("invalid Avro type object: missing 'type' field")
		}

		if schema.IsPrimitive(typeName) {
			return Normalized{Kind: schema.KindPrimitive, Type: typeName, Raw: typeVal}, nil
		}

		switch typeName {
		case "record":
			fields, _ := v["fields"].([]any)
			return Normalized{Kind: schema.KindRecord, Fields: fields, Raw: typeVal}, nil
		case "array":
			return Normalized{Kind: schema.KindArray, Items: v["items"], Raw: typeVal}, nil
		case "map":
			return Normalized{Kind: schema.KindMap, Values: v["values"], Raw: typeVal}, nil
		case "enum":
			return Normalized{Kind: schema.KindEnum, Raw: typeVal}, nil
		case "fixed":
			return Normalized{Kind: schema.KindFixed, Raw: typeVal}, nil
		default:
			return Normalized{}, fmt.Errorf("unknown Avro type: %s", typeName)
		}

	default:
		return Normalized{}, fmt.Errorf("invalid Avro type value: %T", typeVal)
	}
}
