// Package registry implements a client for Confluent-compatible Schema
// Registry REST APIs (Confluent Cloud, self-hosted Confluent Schema
// Registry, and compatible layers such as Apicurio/AWS Glue).
//
// Only read operations plus the compatibility-check endpoint are supported;
// there is no schema registration/push in this client by design (see
// docs/ARCHITECTURE.md "Schema Registry").
package registry

import "fmt"

// SchemaVersion represents a single registered version of a subject.
type SchemaVersion struct {
	Subject    string `json:"subject"`
	Version    int    `json:"version"`
	ID         int    `json:"id"`
	Schema     string `json:"schema"`     // raw JSON schema text, as stored in the registry
	SchemaType string `json:"schemaType"` // "AVRO" if present; older registries omit this
}

// CompatibilityResult is the outcome of a compatibility check.
type CompatibilityResult struct {
	Compatible bool     `json:"is_compatible"`
	Messages   []string `json:"messages,omitempty"` // per-failure explanations; populated via "?verbose=true" on an incompatible result
}

// APIError represents a non-2xx response from the registry, carrying the
// HTTP status and registry-specific error code/message when available so
// callers can distinguish e.g. "404 subject not found" from "401
// unauthorized" without parsing strings.
type APIError struct {
	StatusCode int
	ErrorCode  int    `json:"error_code"`
	Message    string `json:"message"`
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("registry: %s (http %d, code %d)", e.Message, e.StatusCode, e.ErrorCode)
	}
	return fmt.Sprintf("registry: request failed (http %d)", e.StatusCode)
}

// IsNotFound reports whether the error represents a 404 (unknown subject/
// version/schema id).
func (e *APIError) IsNotFound() bool { return e.StatusCode == 404 }

// IsUnauthorized reports whether the error represents a 401/403 (bad or
// missing credentials).
func (e *APIError) IsUnauthorized() bool { return e.StatusCode == 401 || e.StatusCode == 403 }
