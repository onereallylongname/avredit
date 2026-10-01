# avredit — Avro Editor TUI (Checkpoint v2)

## 🎯 Goal

Port the existing Avro schema web editor into a lazygit-inspired Terminal UI (TUI) called `avredit` with strong usability, fast performance, and cross-platform distribution.

Source editor: <https://onereallylongname.github.io/tools/avro/avro-editor.html>
lazygit: <https://github.com/jesseduffield/lazygit>

---

## 🔧 Technical Decisions

| Decision     | Choice                                               |
| ------------ | ---------------------------------------------------- |
| Framework    | bubbletea + lipgloss                                 |
| Go version   | 1.23+                                                |
| Testing      | Table-driven with testify                            |
| Architecture | Hybrid (mutable projection + pure command factories) |
| Layout       | 4 panels (tree, details, file explorer, status bar)  |
| Modes        | Normal, Edit, Search, Command                        |
| Theming      | Config with runtime switching                        |
| Binary name  | `avredit`                                             |
| Module path  | `github.com/onereallylongname/avredit`                |
| File I/O     | CLI arg + TUI file picker                            |

---

## 🧠 Core Design Principle (Critical)

The system is built around an **internal projection model (source of truth)**:

- The schema is parsed into an internal structured representation
- All manipulations operate ONLY on this projection via **Command** objects
- UI rendering reads from this projection
- Export reconstructs schema from the projection

### Architecture: Hybrid Functional/Mutable

- **Projection** is a mutable container (node map + root ID)
- **Commands** are pure factory functions: `func(projection, params) -> (Command, error)`
- **Command** holds `Do()` and `Undo()` closures that mutate projection in-place
- **Access** to projection is through pure getter functions
- **Bubbletea Model.Update()** calls `cmd.Do()` and returns updated model

### ✅ Implications

- Decouples UI from schema format
- Enables support for multiple formats (future)
- Enables safe transformations and undo/redo
- Simplifies validation pipeline addition later
- Commands are unit-testable without TUI
- No full-tree copy per mutation (performant for large schemas)

---

## 🧭 UX / UI Model

### Layout (4 panels)

```
┌─────────────────────────────────────────────────────┐
├────────────────┬────────────────┬───────────────────┤
│                │                │                   │
│ File Explorer  │  Schema Tree   │  Node Details     │
│ Schema Connect │  (left panel)  │  (center panel)   │
│ (right, stub)  │                │                   │
├────────────────┴────────────────┴───────────────────┤
│ [Command/Search bar + stats: fields, depth, undo]   │
└─────────────────────────────────────────────────────┘
```

- Left panel: file explorer and schema registry connections
- Center panel: schema tree (primary focus)
- Right panel: node details / editing
- Bottom: command/search input + statistics, status bar (mode, schema name)

## ⚠️ Risks & Challenges

- State complexity due to multiple modes
- Managing multi-selection correctly
- Avoiding UI clutter
- Registry integration complexity

## Direction

A terminal-based Avro editor (`avredit`) focused on:

- Strong internal projection model (pure, testable)
- High productivity UX (vim bindings, modes, fast navigation)
- Clear extensibility (commands, themes, future registry)
- Functional Go with hybrid mutable/pure architecture
- bubbletea + lipgloss for composable, testable TUI
- Cross-platform single-binary distribution

---

## Next steps

### Core

These should implemented first as they are foundational.

_Docs_:

- avredit_modular_architecture.md
- go_cli_parser_comparison.md

[ ] **Better cli arguments**
[ ] **Modular application**

### Completed

[x] **Schema Validation (~300-400 LoC)** — Advisory validation integrated
  - Duplicate field names within records
  - Duplicate branches in unions
  - `:validate` command with error/warning counts
  - `:problems` / `:diagnostics` — navigable picker listing all issues (errors first, then warnings)
  - Selecting an issue jumps to the affected node in the tree
  - Validation runs on every mutation (advisory, never blocks)

[x] **Structured Logging**
  - `internal/logging/` package (slog + lumberjack rotation, 10MB max)
  - Config `log_level` field (debug/info/warn/error, default: error)
  - CLI `--log-level` flag (overrides config)
  - Runtime `:log-level` command with picker
  - All internal errors logged with structured context (file paths, node IDs, error details)
  - Silent failures in config/theme loading now emit slog.Warn

[x] **Theme Symbols System**
  - All UI icons extracted into `Symbols` struct (expanded, collapsed, leaf, cursor, error, warning, etc.)
  - Themes can override any/all symbols via JSON `"symbols"` section
  - Defaults used for omitted values (graceful degradation)
  - 3 bundled themes with custom symbols (gruvbox ASCII, dracula dramatic, catppuccin cute)

### Features

These are a TODO list and in no particular order.

_Docs_:

- avredit_validation_module.md
- avredit_tui_validation_baseline.md

[ ] **Schema Registry Integration (~1500-2000 LoC, 16-24h)**

_Work estimation_:

- New package `internal/registry/` — HTTP client for Schema Registry REST API (~400 LoC)
- Auth handling: basic auth, API key, mTLS (~200 LoC)
- Commands: `:registry connect/push/pull/compat/list` (~300 LoC)
- Registry browser overlay (list subjects, versions) (~300 LoC)
- Config extension for registry URLs/credentials (~100 LoC)
- Compatibility check result display (~100 LoC)
- Tests (~300 LoC)

[ ] **Diff View (~300-400 LoC, 4-5h)**

- Store "last saved" serialized snapshot in App state (~20 LoC)
- Serialize current state and compute line-by-line diff (~100 LoC)
- New `:diff` command + `OverlayDiff` overlay type (~50 LoC)
- Colored diff renderer (+green/-red/~yellow) with scroll (~150 LoC)
- Tests (~80 LoC)

### Other

[ ] **awesome-go listing (0 LoC, 30min)**

- Submit PR to github.com/avelino/awesome-go under "Data Structures and Algorithms" or "Command Line" category
- Requires: project has tests ✅, CI ✅, README ✅, godoc ✅, >30 days old (check)
