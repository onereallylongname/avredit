package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultTimeout is used when Config.Timeout is zero.
const DefaultTimeout = 10 * time.Second

// Config configures a Client.
type Config struct {
	// URL is the registry base URL, e.g. "https://psrc-xxxxx.region.aws.confluent.cloud"
	URL string

	AuthType AuthType
	Username string
	Password string
	Token    string

	// Timeout bounds every request. Defaults to DefaultTimeout if zero.
	Timeout time.Duration
}

// Client talks to a Confluent-compatible Schema Registry REST API.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient builds a Client from Config.
func NewClient(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &Client{
		baseURL: strings.TrimRight(cfg.URL, "/"),
		http: &http.Client{
			Timeout: timeout,
			Transport: &authTransport{
				Type:     cfg.AuthType,
				Username: cfg.Username,
				Password: cfg.Password,
				Token:    cfg.Token,
			},
		},
	}
}

// ListSubjects returns all subject names registered in the registry.
func (c *Client) ListSubjects(ctx context.Context) ([]string, error) {
	var subjects []string
	if err := c.doJSON(ctx, http.MethodGet, "/subjects", nil, &subjects); err != nil {
		return nil, err
	}
	return subjects, nil
}

// ListVersions returns all version numbers registered for a subject.
func (c *Client) ListVersions(ctx context.Context, subject string) ([]int, error) {
	var versions []int
	path := "/subjects/" + url.PathEscape(subject) + "/versions"
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &versions); err != nil {
		return nil, err
	}
	return versions, nil
}

// GetSchema fetches a specific version of a subject. Pass version <= 0 to
// fetch "latest".
func (c *Client) GetSchema(ctx context.Context, subject string, version int) (*SchemaVersion, error) {
	v := "latest"
	if version > 0 {
		v = strconv.Itoa(version)
	}
	path := "/subjects/" + url.PathEscape(subject) + "/versions/" + v
	var sv SchemaVersion
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &sv); err != nil {
		return nil, err
	}
	return &sv, nil
}

// schemaByIDResponse matches the GET /schemas/ids/{id} response shape, which
// omits subject/version (those are only known via the subjects/versions
// endpoints).
type schemaByIDResponse struct {
	Schema     string `json:"schema"`
	SchemaType string `json:"schemaType"`
}

// GetSchemaByID fetches a schema by its global registry ID.
func (c *Client) GetSchemaByID(ctx context.Context, id int) (*SchemaVersion, error) {
	path := "/schemas/ids/" + strconv.Itoa(id)
	var resp schemaByIDResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &SchemaVersion{ID: id, Schema: resp.Schema, SchemaType: resp.SchemaType}, nil
}

// compatibilityRequest is the request body for the compatibility endpoint.
type compatibilityRequest struct {
	Schema     string `json:"schema"`
	SchemaType string `json:"schemaType,omitempty"`
}

// compatibilityResponse matches the registry's raw response shape. Messages
// is only populated when the request is made with "?verbose=true" (see
// CheckCompatibility), per the Confluent Schema Registry API.
type compatibilityResponse struct {
	IsCompatible bool     `json:"is_compatible"`
	Messages     []string `json:"messages,omitempty"`
}

// CheckCompatibility checks whether schemaJSON is compatible with the given
// subject/version (version <= 0 means "latest") according to the registry's
// configured compatibility mode for that subject. The request always asks
// for "verbose=true" so that, on an incompatible result, the registry
// explains exactly which check(s) failed instead of just returning false.
func (c *Client) CheckCompatibility(ctx context.Context, subject string, version int, schemaJSON string) (*CompatibilityResult, error) {
	v := "latest"
	if version > 0 {
		v = strconv.Itoa(version)
	}
	path := "/compatibility/subjects/" + url.PathEscape(subject) + "/versions/" + v + "?verbose=true"
	body := compatibilityRequest{Schema: schemaJSON, SchemaType: "AVRO"}
	var resp compatibilityResponse
	if err := c.doJSON(ctx, http.MethodPost, path, body, &resp); err != nil {
		return nil, err
	}
	return &CompatibilityResult{Compatible: resp.IsCompatible, Messages: resp.Messages}, nil
}

// doJSON performs an HTTP request with an optional JSON body and decodes a
// JSON response into out (if non-nil).
func (c *Client) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("registry: encoding request: %w", err)
		}
		bodyReader = strings.NewReader(string(data))
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("registry: building request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.schemaregistry.v1+json, application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/vnd.schemaregistry.v1+json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("registry: request failed: %w", err)
	}
	defer resp.Body.Close()

	data, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return fmt.Errorf("registry: reading response: %w", readErr)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := &APIError{StatusCode: resp.StatusCode}
		_ = json.Unmarshal(data, apiErr) // best-effort; body may not be JSON
		return apiErr
	}

	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("registry: decoding response: %w", err)
	}
	return nil
}
