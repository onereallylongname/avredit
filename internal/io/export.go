package io

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/onereallylongname/avredit/internal/projection"
)

// ExportAvroToFile serializes the projection to a pretty-printed .avsc file.
func ExportAvroToFile(proj *projection.Projection, path string) error {
	data, err := SerializeAvro(proj, true)
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}
	return nil
}

// ExportAvroToClipboard serializes the projection and copies it to the system clipboard.
// Returns the serialized string for confirmation or fallback display.
func ExportAvroToClipboard(proj *projection.Projection) (string, error) {
	data, err := SerializeAvro(proj, true)
	if err != nil {
		return "", err
	}

	text := string(data)

	if err := writeClipboard(text); err != nil {
		return text, fmt.Errorf("clipboard write failed: %w", err)
	}

	return text, nil
}

// SerializeAvro converts the projection back to Avro JSON bytes.
func SerializeAvro(proj *projection.Projection, pretty bool) ([]byte, error) {
	avroVal, err := projection.GenerateAvro(proj)
	if err != nil {
		return nil, fmt.Errorf("generating avro: %w", err)
	}

	if pretty {
		return json.MarshalIndent(avroVal, "", "  ")
	}
	return json.Marshal(avroVal)
}
