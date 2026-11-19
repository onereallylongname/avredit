# avredit — Avro Editor TUI (Checkpoint v2)

## 🎯 Goal

Port the existing Avro schema web editor into a lazygit-inspired Terminal UI (TUI) called `avredit` with strong usability, fast performance, and cross-platform distribution.

Source editor: https://onereallylongname.github.io/tools/avro/avro-editor.html
lazygit: https://github.com/jesseduffield/lazygit

---

## 🔧 Technical Decisions

| Decision     | Choice                                                   |
| ------------ | -------------------------------------------------------- |
| Framework    | bubbletea + lipgloss                                     |
| Go version   | 1.23+                                                    |
| Testing      | Table-driven with testify                                |
| Architecture | Hybrid (mutable projection + pure command factories)     |
| Layout       | 4 panels (tree, details, file explorer stub, status bar) |
| Modes        | Normal, Edit, Search, Command                            |
| Theming      | TOML config with runtime switching                       |
| Binary name  | `avredit`                                                 |
| Module path  | `github.com/onereallylongname/avredit`                    |
| File I/O     | CLI arg + TUI file picker                                |

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

## 📌 Assumptions

- Schema size is small to medium (interactive use)
- User is a developer comfortable with terminal tools
- Validation is not required initially (extensible placeholder needed)
- Single-user local workflow

---

## ✅ Functional Requirements

### Core (v1)

- Open / import schema files
- Save / export schema files
- Tree-based navigation
- Field editing
- Add / remove / move / duplicate fields
- Undo / redo
- Basic search

### Advanced (v2+)

- Schema registry integration (as source and sink)
- Tree folding and markers
- Advanced search
- Multi-selection and bulk edit
- Custom theming

---

## 🧭 UX / UI Model

### Layout (4 panels)

```
┌─────────────────────────────────────────────────────┐
│ [Status: mode indicator, schema name, help hints]   │
├──────────────────┬──────────────────┬───────────────┤
│                  │                  │               │
│  Schema Tree     │  Node Details    │ File Explorer │
│  (left panel)    │  (center panel)  │ (right, stub) │
│                  │                  │               │
├──────────────────┴──────────────────┴───────────────┤
│ [Command/Search bar + stats: fields, depth, undo]   │
└─────────────────────────────────────────────────────┘
```

- Left panel: schema tree (primary focus)
- Center panel: node details / editing
- Right panel: file explorer (placeholder in v1)
- Top: status bar (mode, schema name)
- Bottom: command/search input + statistics

### Modes

| Mode    | Activation              | Purpose                                 |
| ------- | ----------------------- | --------------------------------------- |
| Normal  | Default / `Esc`         | Tree navigation, actions                |
| Edit    | `Enter` on detail field | Modify node attributes                  |
| Search  | `/`                     | Filter and navigate matches             |
| Command | `:`                     | Extended commands (save, export, theme) |

### Interaction Principles

- Keyboard-first navigation
- Vim-like bindings (j/k/h/l, g/G, /, :, etc.)
- Mode-based interaction (Normal → Edit → Search → Command)
- Visual feedback for current mode and focused element

---

## 🎮 Interaction Model

### Normal Mode (Tree Navigation)

| Key          | Action                                 |
| ------------ | -------------------------------------- |
| `j` / `↓`    | Move down                              |
| `k` / `↑`    | Move up                                |
| `h` / `←`    | Collapse                               |
| `l` / `→`    | Expand                                 |
| `g` / `Home` | Jump to top                            |
| `G` / `End`  | Jump to bottom                         |
| `Space`      | Toggle expand/collapse                 |
| `Enter`      | Open details for editing (→ Edit mode) |

### Actions (Normal Mode)

| Key            | Action                         |
| -------------- | ------------------------------ |
| `a`            | Add sibling field              |
| `d` / `Delete` | Remove node                    |
| `c`            | Copy node                      |
| `m`            | Move field (target picker)     |
| `r`            | Replace type                   |
| `F2`           | Rename (focus name in details) |
| `u`            | Undo                           |
| `Ctrl+R`       | Redo                           |

### Search Mode

| Key   | Action                         |
| ----- | ------------------------------ |
| `/`   | Activate search                |
| `n`   | Next match                     |
| `N`   | Previous match                 |
| `Esc` | Clear search, return to Normal |
| `n:`  | Filter by name                 |
| `t:`  | Filter by type                 |
| `p:`  | Filter by parent               |
| `ns:` | Filter by namespace            |
| `a:`  | Filter by alias                |
| `d:`  | Filter by doc text             |

### Command Mode

| Key             | Action                   |
| --------------- | ------------------------ |
| `:`             | Activate command mode    |
| `:w`            | Save file                |
| `:q`            | Quit                     |
| `:wq`           | Save and quit            |
| `:export`       | Export .avsc             |
| `:theme <name>` | Switch theme             |
| `:new`          | New empty schema         |
| `Esc`           | Cancel, return to Normal |

---

## 🧩 System Architecture

### Package Structure

```
cmd/avredit/main.go          Entry point, CLI arg parsing
internal/
  model/                    Bubbletea models (TUI layer)
    app.go                  Root model, panel routing, mode dispatch
    tree.go                 Tree panel (flatten, render, navigate)
    details.go              Detail panel (view/edit node attributes)
    explorer.go             File explorer panel (stub)
    statusbar.go            Status bar (mode, stats, hints)
    command.go              Command mode input
    search.go               Search mode input + results
  projection/               Core data model (zero TUI dependency)
    projection.go           Projection struct, BuildProjection
    node.go                 Node struct, kinds, attributes
    normalize.go            normalizeType (string|array|object → kind)
    emit.go                 GenerateAvro (projection → JSON)
    clone.go                cloneSubtree
    stats.go                calculateSchemaStats
    validate.go             assertProjectionComplete
  command/                  Mutation commands (pure factories)
    command.go              Command interface, History (undo/redo stack)
    attribute.go            UpdateAttribute
    move.go                 MoveNode
    create.go               CreateNode
    copy.go                 CopyNode
    remove.go               RemoveNode
    replace.go              ReplaceType
  search/                   Query engine (no TUI dependency)
    query.go                queryNodes, scoring, filters
    fuzzy.go                fuzzy match, Levenshtein, similarity
  schema/                   Avro type system
    types.go                Constants (PRIMITIVE_TYPES, COMPLEX_TYPES, etc.)
    validate.go             validateName, validateNamespace, etc.
    templates.go            TYPE_TEMPLATES for new node creation
  io/                       File operations
    load.go                 Parse .avsc from file/reader
    export.go               Serialize + write .avsc
  theme/                    Theming
    theme.go                Theme struct, lipgloss style derivation
    toml.go                 TOML parsing
    defaults.go             Built-in themes (dark, light, nord)
  config/                   Configuration
    config.go               Config struct, keybindings, paths
```

### Core Layers

1. **Input Layer** (`io/`)
   - Parse .avsc files from disk or CLI args
   - Future: schema registry client

2. **Projection Layer** (`projection/`)
   - Internal tree representation
   - Source of truth for all operations
   - Node kinds: schema, record, field, primitive, named, union, array, map, enum, fixed

3. **Transformation Layer** (`command/`)
   - Pure command factories
   - Undo/redo history stack (500 deep)
   - Each command: Do() + Undo() + Description

4. **Rendering Layer** (`model/`)
   - Bubbletea models per panel
   - Reads projection, dispatches commands
   - Mode-aware key handling

5. **Output Layer** (`io/`)
   - Serialize projection → Avro JSON
   - Write to file or clipboard

---

## 🔌 Extensibility Points

- Validation pipeline (pluggable)
- Input/output providers (file, registry)
- Themes
- Commands

---

## 🎨 Theming

- TOML-based theme configuration
- Runtime switching via `:theme <name>` command
- Ship 3 built-in themes: dark (default), light, nord
- Theme defines: background, foreground, accent, node-kind colors, mode indicator colors
- Lipgloss styles derived from theme at load time
- Config file: `~/.config/avredit/config.toml`

---

## 📦 Packaging Strategy

### Distribution Goals

- Zero friction installation
- Cross-platform compatibility

### Package Managers

- macOS: Homebrew
- Linux: apt / snap
- Windows: Scoop

### Distribution Method

- Pre-built binaries via releases

---

## ⚠️ Risks & Challenges

- State complexity due to multiple modes
- Managing multi-selection correctly
- Avoiding UI clutter
- Registry integration complexity

---

## 🚀 Development Strategy

### Phase 1: Foundation (Core Logic, No UI) ✅

- Go module init + dependencies (bubbletea, lipgloss, testify)
- All `projection/`, `command/`, `search/`, `schema/`, `io/` packages
- Full unit test coverage
- Proves the engine works without any TUI code

### Phase 2: TUI Shell (Layout + Navigation) ✅

- Entry point with CLI arg parsing
- 4-panel layout with bubbletea/lipgloss
- Tree rendering from projection
- Normal mode navigation (j/k/h/l/g/G/Space/Enter)
- Default dark theme

### Phase 3: Edit Mode + Actions ✅

- Detail panel with editable fields
- All mutation actions wired (add/delete/copy/move/replace)
- Undo/redo integration
- Status bar shows history depth

### Phase 4: Search + Command Mode ✅

- Search input with fuzzy matching
- Prefix filters (n:, t:, p:, ns:, a:, d:) and free-text matching over doc fields
- Command mode (:w, :q, :export, :theme)
- File picker for opening new schemas

### Phase 5: Theming + Config ✅

- JSON theme files (migrated from TOML)
- 10 built-in themes (dark, light, monokai, catppuccin-mocha, tokyo-night, rose-pine, dracula, gruvbox, one-dark-pro, vscode-light)
- Runtime switching via `:theme` command
- Config at `~/.config/avredit/config.json`
- Theme persistence (saved on switch)

### Phase 6: Polish ✅

- README with installation, usage, keybindings, config samples
- Config file discovery (cwd → executable → ~/.config)
- Sample config and theme files

### Phase 7: File Explorer ✅

- File explorer pane (left of tree)
- Toggle visibility with Ctrl+E
- Recursive file search with auto-expand
- Directory navigation (parent, set-root)
- Open .avsc files from explorer

### Phase 8: Advanced Polish ✅

- Command history (↑/↓ arrows in command mode)
- Search history (↑/↓ arrows in search mode)
- Persistent history saved to `~/.config/avredit/history.json`
- Unsaved changes confirmation on quit and load
- Named type rename propagation
- Alias rename propagation
- Categorized type picker (Primitives/Complex/Named/Aliases)
- Type picker filter mode (/)
- Responsive status bar with proportional truncation

### Phase 9: Security + Feedback ✅

- Removed confirmation from field delete (direct action)
- Added confirmation for quit-with-unsaved and load-if-unsaved
- Notification system (flash messages + scrollable history)
- Help overlay with scrolling (j/k, ?/Esc to toggle)
- File path resolution fixed (saves relative to explorer root)

### Phase 10: Distribution ✅

- goreleaser configuration (.goreleaser.yaml)
- Homebrew tap + Scoop bucket setup
- Pre-built binaries via GitHub releases
- README with installation instructions for all platforms

### Phase 11: Documentation ✅ (Current)

- Architecture documentation (docs/ARCHITECTURE.md)
  - High-level flow and patterns
  - Mermaid architecture diagram + state machine
  - Low-level code references with line numbers
- Updated code comments matching documentation
- Updated README with "How It Works", search prefixes, tips
- Sample config files (config.json, theme, history)

---

## Improvements to Add (priority order)

| # | Feature | LoC estimate | Time estimate | Status |
|---|---------|-------------|---------------|--------|
| 1 | Schema Registry integration — confluent-compatible pull/push/compatibility-check | ~1500-2000 | 16-24h | pending |
| 2 | Schema validation — warn on invalid defaults, duplicate names, circular refs | ~300-400 | 4-6h | pending |
| 3 | GIF/screenshot in README — critical for GitHub discovery | 0 (media) | 1-2h | manual |
| 4 | Diff view — show what changed since last save (`:diff` command) | ~300-400 | 4-5h | pending |
| 5 | awesome-go listing — free discoverability to 130K+ stars audience | 0 (PR) | 30min | pending |
| 6 | Named type suggestions — type pickers include schema-defined types (records, enums, fixed) | ~30 | 30min | ✅ done |

### Cost Breakdown

**1. Schema Registry Integration (~1500-2000 LoC, 16-24h)**
- New package `internal/registry/` — HTTP client for Schema Registry REST API (~400 LoC)
- Auth handling: basic auth, API key, mTLS (~200 LoC)
- Commands: `:registry connect/push/pull/compat/list` (~300 LoC)
- Registry browser overlay (list subjects, versions) (~300 LoC)
- Config extension for registry URLs/credentials (~100 LoC)
- Compatibility check result display (~100 LoC)
- Tests (~300 LoC)

**2. Schema Validation (~300-400 LoC, 4-6h)**
- Extend `projection/validate.go` — duplicate field names within records (~60 LoC)
- Default value type checking (validate default matches declared type) (~100 LoC)
- Circular reference detection (DFS cycle check) (~80 LoC)
- Integrate warnings into details pane (non-blocking indicators) (~80 LoC)
- Tests (~80 LoC)
- Infrastructure exists: `schema/validate.go` already has name/namespace/symbol validators

**4. Diff View (~300-400 LoC, 4-5h)**
- Store "last saved" serialized snapshot in App state (~20 LoC)
- Serialize current state and compute line-by-line diff (~100 LoC)
- New `:diff` command + `OverlayDiff` overlay type (~50 LoC)
- Colored diff renderer (+green/-red/~yellow) with scroll (~150 LoC)
- Tests (~80 LoC)

**5. awesome-go listing (0 LoC, 30min)**
- Submit PR to github.com/avelino/awesome-go under "Data Structures and Algorithms" or "Command Line" category
- Requires: project has tests ✅, CI ✅, README ✅, godoc ✅, >30 days old (check)

**6. Named Type Suggestions (~30 LoC, 30min) ✅**
- Add `proj.NamedTypes()` results to type picker lists (already implemented)

---

## ✅ Final Direction

A terminal-based Avro editor (`avredit`) focused on:

- Strong internal projection model (pure, testable)
- High productivity UX (vim bindings, modes, fast navigation)
- Clear extensibility (commands, themes, future registry)
- Functional Go with hybrid mutable/pure architecture
- bubbletea + lipgloss for composable, testable TUI
- Cross-platform single-binary distribution
