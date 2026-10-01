// Package io handles file loading and exporting of Avro schemas.
package io

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/onereallylongname/avredit/internal/projection"
)

// LoadAvroFromFile reads and parses an .avsc file into a Projection.
func LoadAvroFromFile(path string) (*projection.Projection, map[string]any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	return LoadAvroFromReader(f)
}

// LoadAvroFromReader parses an Avro schema JSON from a reader into a Projection.
func LoadAvroFromReader(r io.Reader) (*projection.Projection, map[string]any, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, fmt.Errorf("reading input: %w", err)
	}

	return LoadAvroFromBytes(data)
}

// LoadAvroFromBytes parses Avro schema JSON bytes into a Projection.
func LoadAvroFromBytes(data []byte) (*projection.Projection, map[string]any, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, nil, fmt.Errorf("parsing JSON: %w", err)
	}

	if _, ok := raw["type"]; !ok {
		return nil, nil, fmt.Errorf("not a valid Avro schema: missing 'type' field")
	}

	proj, err := projection.Build(raw)
	if err != nil {
		return nil, nil, err
	}

	return proj, raw, nil
}
