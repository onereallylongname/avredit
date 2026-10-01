package model

import (
"os"
"path/filepath"
"testing"

"github.com/onereallylongname/avredit/internal/theme"
)

func TestExplorer_FilterExpandsNestedMatches(t *testing.T) {
// Create temp directory structure:
// root/
//   subdir/
//     deep/
//       target.avsc
//   top.avsc
dir := t.TempDir()
deep := filepath.Join(dir, "subdir", "deep")
os.MkdirAll(deep, 0o755)
os.WriteFile(filepath.Join(deep, "target.avsc"), []byte("{}"), 0o644)
os.WriteFile(filepath.Join(dir, "top.avsc"), []byte("{}"), 0o644)

th := theme.Dark()
e := NewExplorerModel(th, dir)
e.visible = true
e.SetSize(80, 40)

// Initially only root-level entries visible (subdir collapsed, top.avsc)
t.Logf("Before filter: %d entries", len(e.entries))
for i, en := range e.entries {
t.Logf("  [%d] %s (dir=%v, expanded=%v)", i, en.Name, en.IsDir, en.Expanded)
}

// Filter for "target" — should expand subdir and deep
e.Filter("target")

t.Logf("After filter: %d entries, %d matches", len(e.entries), len(e.matchIndices))
for i, en := range e.entries {
t.Logf("  [%d] %s (dir=%v, expanded=%v, depth=%d)", i, en.Name, en.IsDir, en.Expanded, en.Depth)
}

// Verify target.avsc is visible
found := false
for _, en := range e.entries {
if en.Name == "target.avsc" {
found = true
break
}
}
if !found {
t.Fatal("target.avsc should be visible after filter expands ancestors")
}

// Verify subdir and deep are expanded
for _, en := range e.entries {
if en.Name == "subdir" && !en.Expanded {
t.Error("subdir should be expanded")
}
if en.Name == "deep" && !en.Expanded {
t.Error("deep should be expanded")
}
}

// Verify match indices point to target.avsc
if len(e.matchIndices) != 1 {
t.Fatalf("expected 1 match, got %d", len(e.matchIndices))
}
matchEntry := e.entries[e.matchIndices[0]]
if matchEntry.Name != "target.avsc" {
t.Errorf("expected match to be target.avsc, got %s", matchEntry.Name)
}
}
