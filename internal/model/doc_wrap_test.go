package model

import (
	"strings"
	"testing"

	"github.com/onereallylongname/avredit/internal/theme"
)

// TestDetailsModel_DocFieldWraps verifies that a long Doc value wraps to fit
// the panel width instead of overflowing/being truncated with "…".
func TestDetailsModel_DocFieldWraps(t *testing.T) {
	proj := buildTestProjection(t)
	th := theme.Dark()
	details := NewDetailsModel(proj, th)
	details.SetSize(40, 20)

	root := proj.Nodes[proj.RootID]
	details.SetNode(root)

	longDoc := strings.Repeat("word ", 40) // 200 chars, far wider than the panel
	root.Attrs.Native.(map[string]any)["doc"] = longDoc
	details.rebuildAttrs()

	view := stripANSI(details.View())
	for _, line := range strings.Split(view, "\n") {
		if len([]rune(line)) > details.wrapWidth() {
			t.Fatalf("line exceeds wrap width (%d): %q", details.wrapWidth(), line)
		}
	}
	if !strings.Contains(view, "word") {
		t.Fatalf("expected doc content to be present, got:\n%s", view)
	}
}

// TestDetailsModel_DocFieldEditingCursorVisible verifies that when editing a
// long Doc value that wraps across many lines, the cursor stays within the
// rendered (height-clipped) window instead of being scrolled/truncated away.
func TestDetailsModel_DocFieldEditingCursorVisible(t *testing.T) {
	proj := buildTestProjection(t)
	th := theme.Dark()
	details := NewDetailsModel(proj, th)
	details.SetSize(40, 10) // small height to force clipping

	root := proj.Nodes[proj.RootID]
	details.SetNode(root)

	longDoc := strings.Repeat("word ", 60)
	root.Attrs.Native.(map[string]any)["doc"] = longDoc
	details.rebuildAttrs()

	// Move cursor to the Doc attribute.
	for i, a := range details.attrs {
		if a.Key == "Doc" {
			details.cursor = i
			break
		}
	}
	if !details.BeginEdit() {
		t.Fatal("expected BeginEdit to succeed on Doc field")
	}
	// Move the edit cursor to the very end of the (long) value.
	details.editor.cursor = len(details.editor.value)

	view := details.View() // should not panic, and should include a cursor render
	innerH := details.height - 2
	lineCount := len(strings.Split(view, "\n"))
	if lineCount > innerH {
		t.Fatalf("expected view to be clipped to %d lines, got %d", innerH, lineCount)
	}
	// The cursor style should appear somewhere in the visible output.
	if !strings.Contains(view, "\x1b[") {
		t.Fatalf("expected styled (cursor) output in view, got:\n%s", stripANSI(view))
	}
}
