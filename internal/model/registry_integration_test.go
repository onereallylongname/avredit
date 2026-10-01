package model

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/onereallylongname/avredit/internal/config"
	"github.com/onereallylongname/avredit/internal/registry"
)

// newTestRegistryServer returns a minimal fake Confluent-compatible registry
// serving one subject ("orders-value") with two versions, plus a
// compatibility endpoint that always reports compatible.
func newTestRegistryServer(t *testing.T) *httptest.Server {
	t.Helper()
	const schemaV1 = `{"type":"record","name":"Order","namespace":"com.example","fields":[{"name":"id","type":"string"}]}`
	const schemaV2 = `{"type":"record","name":"Order","namespace":"com.example","fields":[{"name":"id","type":"string"},{"name":"total","type":"double"}]}`

	mux := http.NewServeMux()
	mux.HandleFunc("/subjects", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]string{"orders-value"})
	})
	mux.HandleFunc("/subjects/orders-value/versions", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]int{1, 2})
	})
	mux.HandleFunc("/subjects/orders-value/versions/1", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"subject": "orders-value", "version": 1, "id": 1, "schema": schemaV1})
	})
	mux.HandleFunc("/subjects/orders-value/versions/2", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"subject": "orders-value", "version": 2, "id": 2, "schema": schemaV2})
	})
	mux.HandleFunc("/subjects/orders-value/versions/latest", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"subject": "orders-value", "version": 2, "id": 2, "schema": schemaV2})
	})
	mux.HandleFunc("/compatibility/subjects/orders-value/versions/latest", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"is_compatible": true})
	})
	return httptest.NewServer(mux)
}

func TestApp_RegistryMode_ToggleFetchesSubjects(t *testing.T) {
	srv := newTestRegistryServer(t)
	defer srv.Close()

	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app = app.WithRegistryClient(registry.NewClient(registry.Config{URL: srv.URL}), "test-registry")
	app.width, app.height = 120, 40
	app.updateLayout()

	if app.registryClient == nil {
		t.Fatal("expected registry client to be attached")
	}

	model, cmd := app.toggleRegistryMode()
	app = model.(App)
	if app.explorer.Mode() != ExplorerRegistry {
		t.Fatalf("expected explorer mode Registry after toggle, got %v", app.explorer.Mode())
	}
	if cmd == nil {
		t.Fatal("expected a subjects-fetch cmd on first switch to Registry mode")
	}

	msg := cmd()
	subjMsg, ok := msg.(registrySubjectsMsg)
	if !ok {
		t.Fatalf("expected registrySubjectsMsg, got %T", msg)
	}
	if subjMsg.err != nil {
		t.Fatalf("unexpected error: %v", subjMsg.err)
	}

	model, _ = app.Update(subjMsg)
	app = model.(App)
	if !app.explorer.HasRegistrySubjects() {
		t.Fatal("expected subjects to be cached after Update")
	}

	// Toggling back to Files mode should not require the registry.
	model, cmd = app.toggleRegistryMode()
	app = model.(App)
	if app.explorer.Mode() != ExplorerFiles {
		t.Fatalf("expected explorer mode Files after second toggle, got %v", app.explorer.Mode())
	}
	if cmd != nil {
		t.Fatal("expected no cmd when switching back to Files mode")
	}
}

func TestApp_RegistryMode_WithoutClientFlashesError(t *testing.T) {
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())

	model, cmd := app.toggleRegistryMode()
	app = model.(App)
	if cmd != nil {
		t.Fatal("expected no cmd when no registry client is configured")
	}
	if app.flashLevel != FlashError || app.flash == "" {
		t.Fatalf("expected an error flash, got level=%v msg=%q", app.flashLevel, app.flash)
	}
	if app.explorer.Mode() != ExplorerFiles {
		t.Fatal("expected explorer mode to remain Files when toggle is rejected")
	}
}

func TestApp_RegistryMode_ExpandFetchesVersionsThenSchema(t *testing.T) {
	srv := newTestRegistryServer(t)
	defer srv.Close()

	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app = app.WithRegistryClient(registry.NewClient(registry.Config{URL: srv.URL}), "test-registry")
	app.width, app.height = 120, 40
	app.updateLayout()

	// Enter registry mode and load subjects synchronously.
	model, cmd := app.toggleRegistryMode()
	app = model.(App)
	model, _ = app.Update(cmd())
	app = model.(App)
	app.focus = PanelExplorer

	// Expand the (only) subject: enter → pending version fetch.
	app.explorer.cursor = 0
	model, cmd = app.updateExplorer(tea.KeyPressMsg{Code: tea.KeyEnter})
	app = model.(App)
	if cmd == nil {
		t.Fatal("expected a versions-fetch cmd after expanding an unfetched subject")
	}
	msg := cmd()
	versMsg, ok := msg.(registryVersionsMsg)
	if !ok {
		t.Fatalf("expected registryVersionsMsg, got %T", msg)
	}
	if versMsg.err != nil {
		t.Fatalf("unexpected error: %v", versMsg.err)
	}
	model, _ = app.Update(versMsg)
	app = model.(App)

	// Select version 2's row and fetch its schema.
	var verIdx = -1
	for i, en := range app.explorer.entries {
		if en.RegVersion == 2 {
			verIdx = i
		}
	}
	if verIdx < 0 {
		t.Fatalf("expected a version 2 row to be visible, entries=%+v", app.explorer.entries)
	}
	app.explorer.cursor = verIdx
	model, cmd = app.updateExplorer(tea.KeyPressMsg{Code: tea.KeyEnter})
	app = model.(App)
	if cmd == nil {
		t.Fatal("expected a schema-fetch cmd after selecting a version row")
	}
	msg = cmd()
	schemaMsg, ok := msg.(registrySchemaMsg)
	if !ok {
		t.Fatalf("expected registrySchemaMsg, got %T", msg)
	}
	if schemaMsg.err != nil {
		t.Fatalf("unexpected error: %v", schemaMsg.err)
	}

	model, _ = app.Update(schemaMsg)
	app = model.(App)
	if app.filePath != "" {
		t.Fatalf("expected filePath to remain empty after loading a registry schema (no push), got %q", app.filePath)
	}
	if app.dirty {
		t.Fatal("expected freshly loaded registry schema to be marked clean")
	}
	if app.proj == nil || app.proj.RootID == "" {
		t.Fatal("expected a loaded projection after registry schema fetch")
	}
}

func TestApp_RegistryCheckCommand(t *testing.T) {
	srv := newTestRegistryServer(t)
	defer srv.Close()

	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app = app.WithRegistryClient(registry.NewClient(registry.Config{URL: srv.URL}), "test-registry")

	model, cmd := app.executeCommand("registry check orders-value")
	app = model.(App)
	if cmd == nil {
		t.Fatal("expected a compatibility-check cmd")
	}
	msg := cmd()
	compatMsg, ok := msg.(registryCompatMsg)
	if !ok {
		t.Fatalf("expected registryCompatMsg, got %T", msg)
	}
	if compatMsg.err != nil {
		t.Fatalf("unexpected error: %v", compatMsg.err)
	}
	if compatMsg.result == nil || !compatMsg.result.Compatible {
		t.Fatalf("expected a compatible result, got %+v", compatMsg.result)
	}

	model, _ = app.Update(compatMsg)
	app = model.(App)
	if app.flashLevel != FlashInfo {
		t.Fatalf("expected an info flash for a compatible result, got level=%v msg=%q", app.flashLevel, app.flash)
	}
}

func TestApp_RegistryCheckCommand_NoClientConfigured(t *testing.T) {
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())

	model, cmd := app.executeCommand("registry check orders-value")
	app = model.(App)
	if cmd != nil {
		t.Fatal("expected no cmd when no registry client is configured")
	}
	if app.flashLevel != FlashError {
		t.Fatalf("expected an error flash, got level=%v msg=%q", app.flashLevel, app.flash)
	}
}

func TestInferRegistrySubject(t *testing.T) {
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	subject, err := app.inferRegistrySubject()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if subject != "com.example.User-value" {
		t.Fatalf("expected inferred subject 'com.example.User-value', got %q", subject)
	}
}

func TestStructuralDiff_FieldLevelChanges(t *testing.T) {
	remote := `{"type":"record","name":"Order","fields":[{"name":"id","type":"string"},{"name":"qty","type":"int"}]}`
	local := `{"type":"record","name":"Order","fields":[{"name":"id","type":"string"},{"name":"total","type":"double"}]}`

	diffs := structuralDiff(remote, local)
	joined := strings.Join(diffs, "\n")
	if !strings.Contains(joined, `removed field "qty"`) {
		t.Errorf("expected diff to mention removed field 'qty', got: %v", diffs)
	}
	if !strings.Contains(joined, `added field "total"`) {
		t.Errorf("expected diff to mention added field 'total', got: %v", diffs)
	}
}

// TestStructuralDiff_SharedFieldTypeChangeOmitted verifies structuralDiff
// stays shallow: a shared field whose (possibly deeply nested) type changed
// produces no diff line at all. Describing that change in full previously
// meant dumping the entire nested record (docs, glossary tags, lineage
// metadata) via fmt.Sprintf("%v", ...) — multi-kilobyte, unreadable lines
// that also made every re-render of the report overlay expensive. The
// registry's own verbose messages (paired in by matchReasonsToDiff) are
// relied on to explain what's actually wrong with a shared field.
func TestStructuralDiff_SharedFieldTypeChangeOmitted(t *testing.T) {
	remote := `{"type":"record","name":"Order","fields":[{"name":"amount","type":"int"}]}`
	local := `{"type":"record","name":"Order","fields":[{"name":"amount","type":"double"}]}`

	diffs := structuralDiff(remote, local)
	if len(diffs) != 0 {
		t.Errorf("expected no diff lines for a shared field's type change, got: %v", diffs)
	}
}

// TestFilterErrorMessages_DropsContextOnlyMessages verifies verbose
// compatibility messages that are just echoed request/schema context
// (oldSchema, oldSchemaVersion, validateFields/compatibility settings) are
// dropped from the report, leaving only messages that actually explain an
// incompatibility.
func TestFilterErrorMessages_DropsContextOnlyMessages(t *testing.T) {
	messages := []string{
		`{oldSchema: <truncated> '{"type":"record","name":"Order"...'}`,
		`{errorType:"READER_FIELD_MISSING_DEFAULT_VALUE", description:"the field 'qty' has no default value", additionalInfo:"qty"}`,
		`{oldSchemaVersion: 2}`,
		`{validateFields: 'true', compatibility: 'FORWARD_TRANSITIVE'}`,
	}
	out := filterErrorMessages(messages)
	if len(out) != 1 {
		t.Fatalf("expected exactly 1 message to survive filtering, got %d: %v", len(out), out)
	}
	if !strings.Contains(out[0], "errorType") {
		t.Errorf("expected the surviving message to be the errorType one, got %q", out[0])
	}
}

// TestTruncateReason_CapsVeryLongMessages verifies an oversized verbose
// message (e.g. one embedding a large chunk of schema JSON) is capped for
// display rather than rendered/saved in full, which previously produced
// unreadable multi-kilobyte report lines.
func TestTruncateReason_CapsVeryLongMessages(t *testing.T) {
	long := strings.Repeat("x", maxReasonDisplayLen+500)
	got := truncateReason(long)
	if len([]rune(got)) >= len([]rune(long)) {
		t.Errorf("expected truncated message to be shorter than the original (%d chars), got %d chars", len([]rune(long)), len([]rune(got)))
	}
	if !strings.Contains(got, "more chars") {
		t.Errorf("expected a truncation note, got %q", got)
	}
	short := "short message"
	if truncateReason(short) != short {
		t.Errorf("expected a short message to be returned unchanged, got %q", truncateReason(short))
	}
}

// TestRenderRegistryReportOverlay_WrapsLongLines verifies a long reason line
// is wrapped to fit within the terminal width rather than overflowing it,
// which previously broke the overlay's layout on realistic (long) verbose
// messages.
func TestRenderRegistryReportOverlay_WrapsLongLines(t *testing.T) {
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.width = 60
	app.height = 30
	longMsg := "reader field 'age' " + strings.Repeat("has no default value and is missing in the old schema ", 5)
	app.lastRegistryCheck = newRegistryCheckReport(
		"com.example.User-value",
		&registry.CompatibilityResult{Compatible: false, Messages: []string{longMsg}},
		[]string{`- removed field "age"`},
	)

	out := app.renderRegistryReportOverlay()
	for _, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > app.width {
			t.Errorf("overlay line exceeds terminal width %d (got %d): %q", app.width, w, line)
		}
	}
}
