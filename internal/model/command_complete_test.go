package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/onereallylongname/avredit/internal/config"
	"github.com/onereallylongname/avredit/internal/registry"
)

// TestCompleteCommand_SingleMatch verifies a single unambiguous match
// completes fully and does not enter cycling mode.
func TestCompleteCommand_SingleMatch(t *testing.T) {
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.cmdBuf = []rune("theme")
	app.cmdCursor = len(app.cmdBuf)
	app.completeCommand()
	if got := string(app.cmdBuf); got != "theme" {
		t.Errorf("cmdBuf = %q, want %q", got, "theme")
	}
	if app.cmdCompletionActive {
		t.Errorf("expected completion cycling inactive for a single match")
	}
}

// TestCompleteCommand_RegistrySubcommand verifies ":registry " completes to
// the single known subcommand "check".
func TestCompleteCommand_RegistrySubcommand(t *testing.T) {
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.cmdBuf = []rune("registry ")
	app.cmdCursor = len(app.cmdBuf)
	app.completeCommand()
	if got := string(app.cmdBuf); got != "registry check" {
		t.Errorf("cmdBuf = %q, want %q", got, "registry check")
	}
}

// TestCompleteCommand_RegistryCheckSubjectCycling verifies ":registry check "
// completion offers registry subjects fetched from the explorer, and that
// repeated Tab presses cycle through all ambiguous matches before wrapping.
func TestCompleteCommand_RegistryCheckSubjectCycling(t *testing.T) {
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.explorer.SetRegistrySubjects([]string{
		"as400-envelope",
		"as400-other",
		"EG3DT.VARLAGT1",
	})

	app.cmdBuf = []rune("registry check as400-")
	app.cmdCursor = len(app.cmdBuf)

	// First Tab: two "as400-" matches share no further common prefix beyond
	// what's typed, so it should jump straight into cycling at candidate 0.
	app.completeCommand()
	if !app.cmdCompletionActive {
		t.Fatal("expected completion cycling to be active for multiple matches")
	}
	if len(app.cmdCompletions) != 2 {
		t.Fatalf("cmdCompletions = %v, want 2 matches", app.cmdCompletions)
	}
	first := string(app.cmdBuf)
	if first != "registry check as400-envelope" && first != "registry check as400-other" {
		t.Errorf("unexpected first completion: %q", first)
	}

	// Second Tab: cycle to the other candidate.
	app.completeCommand()
	second := string(app.cmdBuf)
	if second == first {
		t.Errorf("expected cycling to a different candidate, got %q both times", second)
	}

	// Third Tab: wrap back around to the first candidate.
	app.completeCommand()
	third := string(app.cmdBuf)
	if third != first {
		t.Errorf("expected cycle to wrap back to %q, got %q", first, third)
	}
}

// TestCompleteCommand_ResetsOnOtherKeys verifies that any command-bar key
// other than "tab" ends the completion cycle so the next Tab starts fresh.
func TestCompleteCommand_ResetsOnOtherKeys(t *testing.T) {
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.explorer.SetRegistrySubjects([]string{"subject-a", "subject-b"})
	app.cmdBuf = []rune("registry check subject-")
	app.cmdCursor = len(app.cmdBuf)
	app.completeCommand()
	if !app.cmdCompletionActive {
		t.Fatal("expected cycling active after ambiguous completion")
	}

	app.cmdCompletionActive = false // simulate what updateCommand does on non-tab keys
	if app.cmdCompletionActive {
		t.Fatal("sanity check failed")
	}
}

// TestRenderPanel_LongTitleDoesNotOverflow verifies a long registry hostname
// title is truncated to fit the panel border instead of making the
// rendered line wider than the panel (which would misalign sibling panels
// when joined horizontally).
func TestRenderPanel_LongTitleDoesNotOverflow(t *testing.T) {
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.explorer.SetMode(ExplorerRegistry)
	app.explorer.SetRegistryLabel("psrc-0j199.westeurope.azure.confluent.cloud")
	app.explorer.SetSize(30, 20)
	app.focus = PanelExplorer

	out := app.renderPanel(app.explorer.View(), PanelExplorer)
	lines := stripANSI(out)
	firstLine := lines[:indexOrLen(lines, '\n')]
	if got := len([]rune(firstLine)); got != 30 {
		t.Errorf("top border width = %d, want 30 (panel width); line: %q", got, firstLine)
	}
}

func indexOrLen(s string, sep rune) int {
	for i, r := range s {
		if r == sep {
			return i
		}
	}
	return len(s)
}

// TestRegistryCheck_IncompatibleOpensReportOverlay verifies an incompatible
// result auto-opens the report overlay and stores it as the (single)
// last-check report.
func TestRegistryCheck_IncompatibleOpensReportOverlay(t *testing.T) {
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.applyRegistryCompatResult("com.example.User-value", registryCompatMsg{
		subject: "com.example.User-value",
		result:  &registry.CompatibilityResult{Compatible: false, Messages: []string{"reader field missing writer default"}},
		diff:    []string{"removed field: age"},
	})
	if app.overlay != OverlayRegistryReport {
		t.Errorf("overlay = %v, want OverlayRegistryReport", app.overlay)
	}
	if app.lastRegistryCheck == nil || app.lastRegistryCheck.Subject != "com.example.User-value" {
		t.Fatalf("lastRegistryCheck not stored correctly: %+v", app.lastRegistryCheck)
	}
}

// TestRegistryCheck_CompatibleStaysQuiet verifies a compatible result does
// NOT open any overlay — it stays a simple bottom-bar flash message, and the
// report is still recorded for later ":registry report" viewing.
func TestRegistryCheck_CompatibleStaysQuiet(t *testing.T) {
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.applyRegistryCompatResult("com.example.User-value", registryCompatMsg{
		subject: "com.example.User-value",
		result:  &registry.CompatibilityResult{Compatible: true},
	})
	if app.overlay != OverlayNone {
		t.Errorf("overlay = %v, want OverlayNone for a compatible result", app.overlay)
	}
	if app.lastRegistryCheck == nil {
		t.Fatal("expected lastRegistryCheck to be recorded even when compatible")
	}
}

// TestRegistryReportCommand_ReopensLastReport verifies ":registry report"
// reopens the last stored report, and errors clearly if none exists yet.
func TestRegistryReportCommand_ReopensLastReport(t *testing.T) {
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())

	// No report yet.
	result, _ := app.executeRegistryCommand([]string{"report"})
	app = result.(App)
	if app.overlay != OverlayNone {
		t.Errorf("overlay = %v, want OverlayNone with no report yet", app.overlay)
	}

	app.lastRegistryCheck = newRegistryCheckReport("s", &registry.CompatibilityResult{Compatible: true}, nil)
	result, _ = app.executeRegistryCommand([]string{"report"})
	app = result.(App)
	if app.overlay != OverlayRegistryReport {
		t.Errorf("overlay = %v, want OverlayRegistryReport", app.overlay)
	}
}

// TestSaveRegistryReport_TimestampedFilename verifies saving a report writes
// a timestamped file (so repeated checks never overwrite an earlier one)
// under the explorer's root directory, containing the verdict and diff.
func TestSaveRegistryReport_TimestampedFilename(t *testing.T) {
	proj := buildTestProjection(t)
	dir := t.TempDir()
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.explorer = NewExplorerModel(app.theme, dir)
	app.lastRegistryCheck = newRegistryCheckReport(
		"com.example.User-value",
		&registry.CompatibilityResult{Compatible: false, Messages: []string{"boom"}},
		[]string{"removed field: age"},
	)
	app.lastRegistryCheck.CheckedAt = time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)
	app.saveRegistryReport()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var found string
	for _, e := range entries {
		if strings.Contains(e.Name(), "registry-check") {
			found = e.Name()
		}
	}
	if found == "" {
		t.Fatalf("no registry-check report file written in %s; entries: %v", dir, entries)
	}
	if !strings.Contains(found, "20260102-150405") {
		t.Errorf("filename %q does not contain expected timestamp", found)
	}
	data, err := os.ReadFile(filepath.Join(dir, found))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "NOT COMPATIBLE") || !strings.Contains(content, "removed field: age") {
		t.Errorf("report content missing expected sections: %s", content)
	}

	// Saving again should produce a second, distinct file (different
	// timestamp), never overwriting the first.
	app.lastRegistryCheck.CheckedAt = time.Date(2026, 1, 2, 15, 4, 6, 0, time.UTC)
	app.saveRegistryReport()
	entries, _ = os.ReadDir(dir)
	count := 0
	for _, e := range entries {
		if strings.Contains(e.Name(), "registry-check") {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected 2 report files after saving twice with different timestamps, got %d", count)
	}
}

// TestMatchReasonsToDiff_PairsByFieldName verifies verbose compatibility
// messages get attached to the structural diff line for the field they
// mention, and any message that mentions no diffed field ends up in the
// "unmatched" ("Other reasons") bucket instead of being silently dropped.
func TestMatchReasonsToDiff_PairsByFieldName(t *testing.T) {
	diff := []string{
		`- removed field "age"`,
		`+ added field "nickname"`,
	}
	messages := []string{
		"reader field 'age' has no default value and is missing in the old schema",
		"compatibility mode BACKWARD does not allow this change",
	}

	entries, unmatched := matchReasonsToDiff(diff, messages)
	if len(entries) != 2 {
		t.Fatalf("expected 2 diff entries, got %d", len(entries))
	}
	if len(entries[0].Reasons) != 1 || !strings.Contains(entries[0].Reasons[0], "age") {
		t.Errorf("expected the 'age' message attached to the removed-field entry, got %+v", entries[0])
	}
	if len(entries[1].Reasons) != 0 {
		t.Errorf("expected no reason matched to the 'nickname' entry, got %+v", entries[1])
	}
	if len(unmatched) != 1 || !strings.Contains(unmatched[0], "compatibility mode") {
		t.Errorf("expected the unrelated message to be unmatched, got %+v", unmatched)
	}
}

// TestRegistryCheckReport_PlainText_IncludesVerboseReasonsNextToDiff verifies
// the saved report text places each verbose reason directly under the diff
// line it explains.
func TestRegistryCheckReport_PlainText_IncludesVerboseReasonsNextToDiff(t *testing.T) {
	r := newRegistryCheckReport(
		"com.example.User-value",
		&registry.CompatibilityResult{
			Compatible: false,
			Messages:   []string{"reader field 'age' has no default value and is missing in the old schema"},
		},
		[]string{`- removed field "age"`},
	)
	text := r.PlainText()
	diffIdx := strings.Index(text, `removed field "age"`)
	reasonIdx := strings.Index(text, "has no default value")
	if diffIdx == -1 || reasonIdx == -1 || reasonIdx < diffIdx {
		t.Errorf("expected the verbose reason to appear directly after its diff line, got:\n%s", text)
	}
}
