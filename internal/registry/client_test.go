package registry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestListSubjects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/subjects" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode([]string{"orders-value", "customers-value"})
	}))
	defer srv.Close()

	c := NewClient(Config{URL: srv.URL})
	subjects, err := c.ListSubjects(context.Background())
	if err != nil {
		t.Fatalf("ListSubjects: %v", err)
	}
	if len(subjects) != 2 || subjects[0] != "orders-value" {
		t.Errorf("unexpected subjects: %+v", subjects)
	}
}

func TestListVersions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/subjects/orders-value/versions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode([]int{1, 2, 3})
	}))
	defer srv.Close()

	c := NewClient(Config{URL: srv.URL})
	versions, err := c.ListVersions(context.Background(), "orders-value")
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 3 || versions[2] != 3 {
		t.Errorf("unexpected versions: %+v", versions)
	}
}

func TestListVersions_SubjectNeedsEscaping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/subjects/a%2Fb/versions" && r.URL.EscapedPath() != "/subjects/a%2Fb/versions" {
			t.Fatalf("expected escaped subject in path, got: %s", r.URL.EscapedPath())
		}
		json.NewEncoder(w).Encode([]int{1})
	}))
	defer srv.Close()

	c := NewClient(Config{URL: srv.URL})
	if _, err := c.ListVersions(context.Background(), "a/b"); err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
}

func TestGetSchema_Latest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/subjects/orders-value/versions/latest" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(SchemaVersion{
			Subject: "orders-value", Version: 3, ID: 42, Schema: `{"type":"record","name":"Order","fields":[]}`,
		})
	}))
	defer srv.Close()

	c := NewClient(Config{URL: srv.URL})
	sv, err := c.GetSchema(context.Background(), "orders-value", 0)
	if err != nil {
		t.Fatalf("GetSchema: %v", err)
	}
	if sv.Version != 3 || sv.ID != 42 {
		t.Errorf("unexpected schema version: %+v", sv)
	}
}

func TestGetSchema_SpecificVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/subjects/orders-value/versions/2" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(SchemaVersion{Subject: "orders-value", Version: 2, ID: 41})
	}))
	defer srv.Close()

	c := NewClient(Config{URL: srv.URL})
	sv, err := c.GetSchema(context.Background(), "orders-value", 2)
	if err != nil {
		t.Fatalf("GetSchema: %v", err)
	}
	if sv.Version != 2 {
		t.Errorf("expected version 2, got %d", sv.Version)
	}
}

func TestGetSchemaByID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/schemas/ids/42" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"schema": `{"type":"string"}`, "schemaType": "AVRO"})
	}))
	defer srv.Close()

	c := NewClient(Config{URL: srv.URL})
	sv, err := c.GetSchemaByID(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetSchemaByID: %v", err)
	}
	if sv.ID != 42 || sv.Schema != `{"type":"string"}` {
		t.Errorf("unexpected result: %+v", sv)
	}
}

func TestCheckCompatibility(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/compatibility/subjects/orders-value/versions/latest" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("verbose") != "true" {
			t.Errorf("expected verbose=true query param, got %q", r.URL.RawQuery)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		json.NewEncoder(w).Encode(map[string]any{"is_compatible": true})
	}))
	defer srv.Close()

	c := NewClient(Config{URL: srv.URL})
	res, err := c.CheckCompatibility(context.Background(), "orders-value", 0, `{"type":"record"}`)
	if err != nil {
		t.Fatalf("CheckCompatibility: %v", err)
	}
	if !res.Compatible {
		t.Error("expected compatible result")
	}
	if gotBody["schema"] != `{"type":"record"}` {
		t.Errorf("unexpected request body: %+v", gotBody)
	}
}

func TestCheckCompatibility_Incompatible(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"is_compatible": false,
			"messages":      []string{"reader field 'age' has no default value and is missing in the old schema"},
		})
	}))
	defer srv.Close()

	c := NewClient(Config{URL: srv.URL})
	res, err := c.CheckCompatibility(context.Background(), "orders-value", 0, `{}`)
	if err != nil {
		t.Fatalf("CheckCompatibility: %v", err)
	}
	if res.Compatible {
		t.Error("expected incompatible result")
	}
	if len(res.Messages) != 1 || res.Messages[0] != "reader field 'age' has no default value and is missing in the old schema" {
		t.Errorf("expected verbose messages to be parsed, got: %+v", res.Messages)
	}
}

func TestNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{"error_code": 40401, "message": "Subject not found"})
	}))
	defer srv.Close()

	c := NewClient(Config{URL: srv.URL})
	_, err := c.GetSchema(context.Background(), "missing", 0)
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if !apiErr.IsNotFound() {
		t.Errorf("expected IsNotFound, got status %d", apiErr.StatusCode)
	}
	if apiErr.Message != "Subject not found" {
		t.Errorf("expected error message from body, got %q", apiErr.Message)
	}
}

func TestUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewClient(Config{URL: srv.URL})
	_, err := c.ListSubjects(context.Background())
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if !apiErr.IsUnauthorized() {
		t.Errorf("expected IsUnauthorized, got status %d", apiErr.StatusCode)
	}
}

func TestBasicAuthHeader(t *testing.T) {
	var gotUser, gotPass string
	var gotOK bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPass, gotOK = r.BasicAuth()
		json.NewEncoder(w).Encode([]string{})
	}))
	defer srv.Close()

	c := NewClient(Config{URL: srv.URL, AuthType: AuthBasic, Username: "alice", Password: "secret"})
	if _, err := c.ListSubjects(context.Background()); err != nil {
		t.Fatalf("ListSubjects: %v", err)
	}
	if !gotOK || gotUser != "alice" || gotPass != "secret" {
		t.Errorf("expected basic auth alice:secret, got ok=%v user=%q pass=%q", gotOK, gotUser, gotPass)
	}
}

func TestBearerAuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode([]string{})
	}))
	defer srv.Close()

	c := NewClient(Config{URL: srv.URL, AuthType: AuthBearer, Token: "abc123"})
	if _, err := c.ListSubjects(context.Background()); err != nil {
		t.Fatalf("ListSubjects: %v", err)
	}
	if gotAuth != "Bearer abc123" {
		t.Errorf("expected 'Bearer abc123', got %q", gotAuth)
	}
}

func TestNoAuthHeader(t *testing.T) {
	var gotAuth string
	var gotOK bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _, gotOK = r.BasicAuth()
		json.NewEncoder(w).Encode([]string{})
	}))
	defer srv.Close()

	c := NewClient(Config{URL: srv.URL})
	if _, err := c.ListSubjects(context.Background()); err != nil {
		t.Fatalf("ListSubjects: %v", err)
	}
	if gotAuth != "" || gotOK {
		t.Errorf("expected no auth header, got Authorization=%q basicAuthOK=%v", gotAuth, gotOK)
	}
}

func TestContextCancellation(t *testing.T) {
	blockCh := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-blockCh
	}))
	defer func() {
		close(blockCh)
		srv.Close()
	}()

	c := NewClient(Config{URL: srv.URL, Timeout: time.Minute})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := c.ListSubjects(ctx)
	if err == nil {
		t.Fatal("expected error due to context cancellation")
	}
}
