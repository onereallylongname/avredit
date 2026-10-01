package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/onereallylongname/avredit/internal/theme"
)

func TestExplorer_RegistryMode_SubjectsAndVersions(t *testing.T) {
	th := theme.Dark()
	e := NewExplorerModel(th, t.TempDir())
	e.visible = true
	e.SetSize(80, 40)

	if e.Mode() != ExplorerFiles {
		t.Fatalf("expected initial mode to be Files, got %v", e.Mode())
	}
	if e.Title() != "Files" {
		t.Fatalf("expected initial title 'Files', got %q", e.Title())
	}

	e.SetRegistryLabel("registry.example.com")
	e.SetMode(ExplorerRegistry)
	if e.Mode() != ExplorerRegistry {
		t.Fatalf("expected mode Registry after SetMode, got %v", e.Mode())
	}
	if got := e.Title(); got != "Registry: registry.example.com" {
		t.Fatalf("expected registry title with label, got %q", got)
	}
	if e.HasRegistrySubjects() {
		t.Fatal("expected no subjects cached yet")
	}

	e.SetRegistrySubjects([]string{"orders-value", "customers-value"})
	if !e.HasRegistrySubjects() {
		t.Fatal("expected subjects to be cached")
	}
	if len(e.entries) != 2 {
		t.Fatalf("expected 2 subject rows, got %d", len(e.entries))
	}
	// SetRegistrySubjects sorts alphabetically.
	if e.entries[0].Name != "customers-value" || e.entries[1].Name != "orders-value" {
		t.Fatalf("expected sorted subject rows, got %+v", e.entries)
	}
	for _, en := range e.entries {
		if !en.IsDir {
			t.Errorf("expected subject row %q to be expandable (IsDir), got IsDir=false", en.Name)
		}
		if en.RegSubject == "" {
			t.Errorf("expected subject row %q to carry RegSubject", en.Name)
		}
	}

	// Selecting the first subject and pressing enter should request a
	// version fetch (no cached versions yet) rather than return an action.
	e.cursor = 0
	action := e.EnterSelected()
	if action.Kind != ExplorerActionNone {
		t.Fatalf("expected no action on first expand (fetch pending), got %+v", action)
	}
	subject, ok := e.TakePendingFetch()
	if !ok || subject != e.entries[0].RegSubject {
		t.Fatalf("expected pending fetch for %q, got (%q, %v)", e.entries[0].RegSubject, subject, ok)
	}
	// Pending fetch is consumed exactly once.
	if _, ok := e.TakePendingFetch(); ok {
		t.Fatal("expected pending fetch to be cleared after TakePendingFetch")
	}

	// Simulate the async versions arriving.
	e.SetRegistryVersions(subject, []int{2, 1, 3})
	found := map[int]bool{}
	for _, en := range e.entries {
		if en.RegSubject == subject && en.RegVersion > 0 {
			found[en.RegVersion] = true
		}
	}
	if !found[1] || !found[2] || !found[3] {
		t.Fatalf("expected versions 1,2,3 to be visible after SetRegistryVersions, entries=%+v", e.entries)
	}

	// Selecting a version row and pressing enter should request a schema fetch.
	for i, en := range e.entries {
		if en.RegSubject == subject && en.RegVersion == 2 {
			e.cursor = i
			break
		}
	}
	action = e.EnterSelected()
	if action.Kind != ExplorerActionFetchSchema {
		t.Fatalf("expected ExplorerActionFetchSchema, got %+v", action)
	}
	if action.Subject != subject || action.Version != 2 {
		t.Fatalf("expected fetch action for %s@2, got %+v", subject, action)
	}
}

func TestExplorer_RegistryMode_FilterIsNetworkFree(t *testing.T) {
	th := theme.Dark()
	e := NewExplorerModel(th, t.TempDir())
	e.visible = true
	e.SetSize(80, 40)
	e.SetMode(ExplorerRegistry)
	e.SetRegistrySubjects([]string{"orders-value", "customers-value", "shipments-value"})

	e.Filter("orders")
	if e.FilteredCount() != 1 {
		t.Fatalf("expected 1 match for 'orders', got %d", e.FilteredCount())
	}
	// Filtering must not have cleared the cached subject list (no network I/O).
	if !e.HasRegistrySubjects() {
		t.Fatal("filtering should not clear cached registry subjects")
	}
}

func TestExplorer_FilesMode_UnaffectedByRegistryFields(t *testing.T) {
	th := theme.Dark()
	e := NewExplorerModel(th, t.TempDir())
	if e.Mode() != ExplorerFiles {
		t.Fatalf("expected default mode Files, got %v", e.Mode())
	}
	// Files-only keys should still work in Files mode.
	e.visible = true
	e.SetSize(80, 40)
	before := len(e.entries)
	e.HandleKey(tea.KeyPressMsg{Code: 'j'})
	if len(e.entries) != before {
		t.Fatalf("navigation should not change entry count in Files mode")
	}
}

func TestExplorer_SelectedDisplayName_NotTruncated(t *testing.T) {
	th := theme.Dark()
	e := NewExplorerModel(th, t.TempDir())
	e.visible = true
	e.SetSize(20, 40) // narrow panel: View() would truncate a long name

	longName := "a-very-long-schema-file-name-that-would-normally-be-truncated.avsc"
	e.entries = []FileEntry{{Name: longName, Path: "/tmp/" + longName}}
	e.cursor = 0

	if got := e.SelectedDisplayName(); got != longName {
		t.Fatalf("expected full untruncated name %q, got %q", longName, got)
	}

	// Sanity check: the panel view itself does truncate at this width.
	view := e.View()
	if strings.Contains(view, longName) {
		t.Fatal("expected the panel view to truncate the long name at this width")
	}
}

func TestExplorer_SelectedDisplayName_RegistryVersionRow(t *testing.T) {
	th := theme.Dark()
	e := NewExplorerModel(th, t.TempDir())
	e.visible = true
	e.SetSize(80, 40)
	e.SetMode(ExplorerRegistry)
	e.SetRegistrySubjects([]string{"a-very-long-subject-name-value"})
	e.cursor = 0
	e.EnterSelected() // expand the subject row, queuing a pending version fetch
	e.SetRegistryVersions("a-very-long-subject-name-value", []int{1})

	for i, en := range e.entries {
		if en.RegVersion == 1 {
			e.cursor = i
		}
	}
	if got, want := e.SelectedDisplayName(), "a-very-long-subject-name-value@v1"; got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
