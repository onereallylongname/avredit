package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/onereallylongname/avredit/internal/theme"
)

func TestExplorer_FilterUsesFuzzyNames(t *testing.T) {
	dir := t.TempDir()
	deep := filepath.Join(dir, "subdir", "deep")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deep, "target.avsc"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	e := NewExplorerModel(theme.Dark(), dir)
	e.Filter("tgt")

	if e.FilteredCount() != 1 {
		t.Fatalf("filtered count = %d, want 1", e.FilteredCount())
	}
	entry := e.entries[e.matchIndices[0]]
	if entry.Name != "target.avsc" {
		t.Fatalf("matched entry = %q, want target.avsc", entry.Name)
	}
}

func TestExplorer_RegistryFilterUsesSubjectName(t *testing.T) {
	e := NewExplorerModel(theme.Dark(), t.TempDir())
	e.SetMode(ExplorerRegistry)
	e.SetRegistrySubjects([]string{"com.example.orders"})
	e.regExpanded["com.example.orders"] = true
	e.SetRegistryVersions("com.example.orders", []int{1})
	e.Filter("cme")

	if e.FilteredCount() != 2 {
		t.Fatalf("filtered count = %d, want subject and version rows", e.FilteredCount())
	}
	for _, index := range e.matchIndices {
		entry := e.entries[index]
		if entry.RegSubject != "com.example.orders" {
			t.Errorf("matched row subject = %q, want com.example.orders", entry.RegSubject)
		}
	}
}
