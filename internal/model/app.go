// Package model implements the bubbletea TUI models for avredit.
//
// Architecture: App is the root model that owns all sub-models (tree, details, explorer,
// statusbar) and orchestrates mode transitions, overlay management, and command execution.
//
// The mode machine (Normal → Edit/Search/Command) controls input dispatch.
// Overlays (picker, confirm, help, notifications) render above the panel layout.
// All mutations flow through the command.History for undo/redo support.
//
// Key areas:
//   - Update(): message dispatch by mode (search input, command input, panel keys)
//   - commitDetailEdit(): applies edits with rename propagation for named types
//   - typeCategoryPicker(): builds categorized type selection overlay
//   - View(): responsive layout with proportional truncation in status bar
//
// See docs/ARCHITECTURE.md "TUI Layer" for detailed line references.
package model

import (
	"fmt"
	"image/color"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/onereallylongname/avredit/internal/clipboard"
	"github.com/onereallylongname/avredit/internal/command"
	"github.com/onereallylongname/avredit/internal/config"
	"github.com/onereallylongname/avredit/internal/io"
	"github.com/onereallylongname/avredit/internal/logging"
	"github.com/onereallylongname/avredit/internal/projection"
	"github.com/onereallylongname/avredit/internal/registry"
	"github.com/onereallylongname/avredit/internal/schema"
	"github.com/onereallylongname/avredit/internal/search"
	"github.com/onereallylongname/avredit/internal/theme"
	"github.com/onereallylongname/avredit/internal/validation"
)

// AppMode represents the current input mode.
type AppMode int

const (
	ModeNormal  AppMode = iota
	ModeEdit    AppMode = iota
	ModeSearch  AppMode = iota
	ModeCommand AppMode = iota
)

// String returns the display name of the mode.
func (m AppMode) String() string {
	switch m {
	case ModeNormal:
		return "NORMAL"
	case ModeEdit:
		return "EDIT"
	case ModeSearch:
		return "SEARCH"
	case ModeCommand:
		return "COMMAND"
	default:
		return "UNKNOWN"
	}
}

// FocusPanel identifies which panel has focus.
type FocusPanel int

const (
	PanelTree     FocusPanel = iota
	PanelDetails  FocusPanel = iota
	PanelExplorer FocusPanel = iota
)

// Overlay identifies which overlay is currently shown.
type Overlay int

const (
	OverlayNone           Overlay = iota
	OverlayPicker         Overlay = iota
	OverlayConfirm        Overlay = iota
	OverlayHelp           Overlay = iota
	OverlayNotify         Overlay = iota
	OverlayRegistryReport Overlay = iota
)

// OverlayAction identifies what triggered an overlay.
type OverlayAction int

const (
	ActionNone        OverlayAction = iota
	ActionAddField    OverlayAction = iota
	ActionReplace     OverlayAction = iota
	ActionDelete      OverlayAction = iota
	ActionSelectAttr  OverlayAction = iota
	ActionMove        OverlayAction = iota
	ActionSelectTheme OverlayAction = iota
	ActionLoad        OverlayAction = iota
	ActionQuit        OverlayAction = iota
	ActionSetLogLevel OverlayAction = iota
	ActionShowProblems OverlayAction = iota
	ActionLoadRegistry OverlayAction = iota
)

// FlashLevel determines the visual style of a flash message.
type FlashLevel int

const (
	FlashInfo    FlashLevel = iota
	FlashWarning FlashLevel = iota
	FlashError   FlashLevel = iota
)

// NotifyEntry represents one notification in the history.
type NotifyEntry struct {
	Message string
	Level   FlashLevel
}

// App is the root bubbletea model orchestrating all panels and modes.
type App struct {
	// Core state
	proj     *projection.Projection
	history  *command.History
	filePath string
	dirty    bool

	// UI state
	mode   AppMode
	focus  FocusPanel
	width  int
	height int
	ready  bool

	// pendingKey holds a leading key of a two-key vim-style sequence
	// (currently just "y" for yank) awaiting its second key. Resolved (and
	// cleared) on the very next keypress in Normal mode, regardless of
	// what that key is — see handleYankKey.
	pendingKey rune

	// Overlays
	overlay       Overlay
	overlayAction OverlayAction
	picker        Picker
	confirm       Confirm

	// Search state
	searchBuf      []rune
	searchCursor   int
	searchResults  []search.Result
	searchIdx      int
	matchedIDs     map[string]bool
	searchExplorer bool // true = searching files, false = searching tree
	searchHistory  []string
	searchHistIdx  int // -1 means not browsing history

	// Command state
	cmdBuf     []rune
	cmdCursor  int
	cmdHistory []string
	cmdHistIdx int // -1 means not browsing history

	// Tab-completion cycling state for the command bar (":registry check
	// <subject>" etc). Reset to inactive on any edit other than repeated
	// "tab" presses, so a fresh Tab always recomputes matches.
	cmdCompletions    []string // full candidate list for the current word, in cycle order
	cmdCompletionIdx  int      // index into cmdCompletions currently applied to cmdBuf
	cmdCompletionPrefix string // buffer text before the word being completed (kept stable across cycles)
	cmdCompletionActive bool

	// Move state (maps picker display name → node ID)
	moveTargets map[string]string

	// Problems state (maps picker display name → node ID)
	problemsMap map[string]string

	// Pending file to open (for confirm-on-load-if-unsaved flow)
	pendingFile string

	// Pending registry fetch (for confirm-on-load-if-unsaved flow)
	pendingRegistrySubject string
	pendingRegistryVersion int

	// Last ":registry check" result (only the most recent is kept — a new
	// check simply replaces it). Viewed via the OverlayRegistryReport
	// overlay (auto-opened on an incompatible result, or reopened anytime
	// with ":registry report") and optionally saved to disk from there.
	lastRegistryCheck  *RegistryCheckReport
	registryReportScrl int

	// Help overlay scroll offset
	helpScroll int

	// Flash message (temporary status bar message)
	flash      string
	flashLevel FlashLevel
	notifyLog  []NotifyEntry
	notifyScrl int

	// Sub-models
	tree      TreeModel
	details   DetailsModel
	explorer  ExplorerModel
	statusbar StatusBarModel

	// Theme
	theme    *theme.Theme
	themeReg *theme.Registry

	// Validation (advisory, never blocks mutations)
	validator        *validation.Analyzer
	validationIssues []validation.Issue

	// Schema registry (nil if not configured for this session)
	registryClient *registry.Client
	registryLabel  string // short label shown in the explorer panel title, e.g. host
}

// WithRegistryClient attaches a schema registry client to the App for this
// session. Credentials are never persisted; the client is built from
// session-only CLI flags in cmd/avredit/main.go. label is a short display
// name (e.g. the registry host) shown in the explorer panel title when in
// Registry mode.
func (a App) WithRegistryClient(client *registry.Client, label string) App {
	a.registryClient = client
	a.registryLabel = label
	a.explorer.SetRegistryLabel(label)
	return a
}

// NewApp creates a new root App model from a loaded projection.
func NewApp(proj *projection.Projection, filePath string, cfg config.Config) App {
	// Resolve to absolute path for reliable save operations
	if filePath != "" {
		if abs, err := filepath.Abs(filePath); err == nil {
			filePath = abs
		}
	}

	// Resolve themes directory: config override → standard discovery
	themesDir := cfg.ThemesDirResolved()
	if themesDir == "" {
		themesDir = theme.ThemesDir()
	}
	reg := theme.NewRegistry(themesDir)

	// Apply configured default theme (fall back to built-in dark)
	th := reg.Get(cfg.Theme)
	if th == nil {
		th = theme.Dark()
	}

	// Load persistent history
	persisted := config.LoadHistory()

	hist := command.NewHistory(500)
	tree := NewTreeModel(proj, th)
	details := NewDetailsModel(proj, th)
	explorer := NewExplorerModel(th, filepath.Dir(filePath))
	statusbar := NewStatusBarModel(th, filePath)
	v := validation.NewAnalyzer()

	app := App{
		proj:          proj,
		history:       hist,
		filePath:      filePath,
		mode:          ModeNormal,
		focus:         PanelTree,
		tree:          tree,
		details:       details,
		explorer:      explorer,
		statusbar:     statusbar,
		theme:         th,
		themeReg:      reg,
		cmdHistory:    persisted.Commands,
		searchHistory: persisted.Searches,
		validator:     v,
	}
	app.runValidation()
	return app
}

// NewAppExplorer creates an App in directory-browsing mode (no file loaded).
func NewAppExplorer(dir string, cfg config.Config) App {
	themesDir := cfg.ThemesDirResolved()
	if themesDir == "" {
		themesDir = theme.ThemesDir()
	}
	reg := theme.NewRegistry(themesDir)

	th := reg.Get(cfg.Theme)
	if th == nil {
		th = theme.Dark()
	}

	// Load persistent history
	persisted := config.LoadHistory()

	// Empty projection as placeholder
	proj := &projection.Projection{Nodes: map[string]*projection.Node{}}
	hist := command.NewHistory(500)
	tree := NewTreeModel(proj, th)
	details := NewDetailsModel(proj, th)
	explorer := NewExplorerModel(th, dir)
	explorer.Toggle() // Start visible
	statusbar := NewStatusBarModel(th, dir)

	return App{
		proj:          proj,
		history:       hist,
		filePath:      "",
		mode:          ModeNormal,
		focus:         PanelExplorer,
		tree:          tree,
		details:       details,
		explorer:      explorer,
		statusbar:     statusbar,
		theme:         th,
		themeReg:      reg,
		cmdHistory:    persisted.Commands,
		searchHistory: persisted.Searches,
		validator:     validation.NewAnalyzer(),
	}
}

// Init implements tea.Model.
func (a App) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.ready = true
		a.updateLayout()
		return a, nil

	case tea.PasteMsg:
		// Bracketed-paste text from the terminal (Shift+Insert, right-click
		// paste, etc.) — route it into whichever editable input is active.
		// A no-op everywhere else.
		a.pasteText(msg.Content)
		return a, nil

	case tea.KeyPressMsg:
		// Clear flash on any key
		a.flash = ""

		// Overlay intercepts all keys when active
		if a.overlay != OverlayNone {
			return a.updateOverlay(msg)
		}

		// Global: esc/ctrl+c always de-escalate mode
		switch msg.String() {
		case "esc", "ctrl+c":
			if a.mode == ModeEdit {
				if a.details.Editing() {
					a.details.CancelEdit()
				}
				a.mode = ModeNormal
				return a, nil
			}
			if a.mode == ModeSearch {
				a.searchBuf = nil
				a.searchCursor = 0
				a.searchResults = nil
				a.searchIdx = 0
				a.matchedIDs = nil
				a.mode = ModeNormal
				return a, nil
			}
			if a.mode == ModeCommand {
				a.cmdBuf = nil
				a.cmdCursor = 0
				a.mode = ModeNormal
				return a, nil
			}
			if a.mode != ModeNormal {
				a.mode = ModeNormal
				return a, nil
			}
			// In normal mode, Esc clears active search highlights
			if a.matchedIDs != nil || a.explorer.FilteredCount() > 0 {
				a.matchedIDs = nil
				a.searchResults = nil
				a.searchIdx = 0
				a.explorer.ClearFilter()
				a.tree.matchedIDs = nil
				return a, nil
			}
		case "tab":
			if a.mode == ModeNormal {
				a.cycleFocus()
				return a, nil
			}
		}

		// Global shortcuts (any panel)
		switch msg.String() {
		case "q":
			if a.mode == ModeNormal {
				if a.dirty {
					a.confirm = NewConfirm("Unsaved changes. Quit anyway?", a.theme)
					a.overlay = OverlayConfirm
					a.overlayAction = ActionQuit
					return a, nil
				}
					return a.quit()
			}
		case "?":
			if a.mode == ModeNormal {
				a.overlay = OverlayHelp
				return a, nil
			}
		case ":":
			if a.mode == ModeNormal {
				a.mode = ModeCommand
				a.cmdHistIdx = -1
				return a, nil
			}
		case "/":
			if a.mode == ModeNormal {
				a.searchExplorer = (a.focus == PanelExplorer)
				a.searchHistIdx = -1
				a.mode = ModeSearch
				return a, nil
			}
		case "ctrl+e":
			if a.mode == ModeNormal {
				a.explorer.Toggle()
				if a.explorer.Visible() {
					a.focus = PanelExplorer
				} else if a.focus == PanelExplorer {
					a.focus = PanelTree
				}
				a.updateLayout()
				return a, nil
			}
		case "ctrl+g":
			if a.mode == ModeNormal {
				return a.toggleRegistryMode()
			}
		case "1":
			if a.mode == ModeNormal {
				a.focusPanel(1)
				return a, nil
			}
		case "2":
			if a.mode == ModeNormal {
				a.focusPanel(2)
				return a, nil
			}
		case "3":
			if a.mode == ModeNormal {
				a.focusPanel(3)
				return a, nil
			}
		case "ctrl+r":
			if a.mode == ModeNormal {
				if a.history.CanRedo() {
					_ = a.history.Redo()
					a.markDirty()
					a.tree.rebuildLines()
					a.syncDetailsToTree()
				}
				return a, nil
			}
		case "ctrl+s":
			if a.mode == ModeNormal {
				return a.doSave()
			}
		case "n":
			if a.mode == ModeNormal {
				if a.searchExplorer && a.explorer.FilteredCount() > 0 {
					a.explorer.NextMatch()
					return a, nil
				} else if len(a.searchResults) > 0 {
					a.searchIdx = (a.searchIdx + 1) % len(a.searchResults)
					a.jumpToSearchResult()
					return a, nil
				}
			}
		case "N":
			if a.mode == ModeNormal {
				if a.searchExplorer && a.explorer.FilteredCount() > 0 {
					a.explorer.PrevMatch()
					return a, nil
				} else if len(a.searchResults) > 0 {
					a.searchIdx--
					if a.searchIdx < 0 {
						a.searchIdx = len(a.searchResults) - 1
					}
					a.jumpToSearchResult()
					return a, nil
				}
			}
		}

		// Search and Command modes intercept all keys
		if a.mode == ModeSearch {
			return a.updateSearch(msg)
		}
		if a.mode == ModeCommand {
			return a.updateCommand(msg)
		}

		// Resolve a pending two-key sequence ("y" + motion) before
		// dispatching to the panel. Any key after "y" is consumed here,
		// including a mismatched one (which just cancels the sequence).
		if a.mode == ModeNormal && a.pendingKey != 0 {
			pending := a.pendingKey
			a.pendingKey = 0
			return a.handleYankKey(pending, msg.String())
		}

		// Delegate to panel-specific handler (each handles its own modes)
		switch a.focus {
		case PanelTree:
			return a.updateTree(msg)
		case PanelDetails:
			return a.updateDetails(msg)
		case PanelExplorer:
			return a.updateExplorer(msg)
		}

	case registrySubjectsMsg:
		a.explorer.SetRegistryLoading(false)
		if msg.err != nil {
			a.flashError("Registry: failed to list subjects: "+msg.err.Error(), "error", msg.err)
			return a, nil
		}
		a.explorer.SetRegistrySubjects(msg.subjects)
		a.flashInfo(fmt.Sprintf("Registry: %d subjects", len(msg.subjects)))
		return a, nil

	case registryVersionsMsg:
		if msg.err != nil {
			a.flashError("Registry: failed to list versions: "+msg.err.Error(), "subject", msg.subject, "error", msg.err)
			return a, nil
		}
		a.explorer.SetRegistryVersions(msg.subject, msg.versions)
		return a, nil

	case registrySchemaMsg:
		if msg.err != nil {
			a.flashError("Registry: failed to fetch schema: "+msg.err.Error(), "subject", msg.subject, "version", msg.version, "error", msg.err)
			return a, nil
		}
		return a.openRegistrySchema(msg.subject, msg.version, msg.schemaJSON)

	case registryCompatMsg:
		if msg.err != nil {
			a.flashError("Registry: compatibility check failed: "+msg.err.Error(), "subject", msg.subject, "error", msg.err)
			return a, nil
		}
		a.applyRegistryCompatResult(msg.subject, msg)
		return a, nil
	}

	return a, nil
}

// updateSearch handles keys in search mode.
func (a App) updateSearch(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		query := string(a.searchBuf)
		if query != "" {
			a.searchHistory = append(a.searchHistory, query)
		}
		a.searchHistIdx = -1
		if a.searchExplorer {
			if a.explorer.FilteredCount() > 0 {
				a.focus = PanelExplorer
			}
		} else if len(a.searchResults) > 0 {
			a.jumpToSearchResult()
		}
		a.mode = ModeNormal
		return a, nil
	case "up":
		if len(a.searchHistory) > 0 {
			if a.searchHistIdx < 0 {
				a.searchHistIdx = len(a.searchHistory) - 1
			} else if a.searchHistIdx > 0 {
				a.searchHistIdx--
			}
			a.searchBuf = []rune(a.searchHistory[a.searchHistIdx])
			a.searchCursor = len(a.searchBuf)
			a.runSearch()
		}
	case "down":
		if a.searchHistIdx >= 0 {
			if a.searchHistIdx < len(a.searchHistory)-1 {
				a.searchHistIdx++
				a.searchBuf = []rune(a.searchHistory[a.searchHistIdx])
				a.searchCursor = len(a.searchBuf)
			} else {
				a.searchHistIdx = -1
				a.searchBuf = nil
				a.searchCursor = 0
			}
			a.runSearch()
		}
	case "left":
		if a.searchCursor > 0 {
			a.searchCursor--
		}
	case "right":
		if a.searchCursor < len(a.searchBuf) {
			a.searchCursor++
		}
	case "home", "ctrl+a":
		a.searchCursor = 0
	case "end", "ctrl+e":
		a.searchCursor = len(a.searchBuf)
	case "ctrl+v", "shift+insert":
		return a.pasteFromNativeClipboard()
	case "backspace":
		if a.searchCursor > 0 {
			a.searchBuf = append(a.searchBuf[:a.searchCursor-1], a.searchBuf[a.searchCursor:]...)
			a.searchCursor--
			a.runSearch()
		}
	case "delete":
		if a.searchCursor < len(a.searchBuf) {
			a.searchBuf = append(a.searchBuf[:a.searchCursor], a.searchBuf[a.searchCursor+1:]...)
			a.runSearch()
		}
	default:
		if !isControlKey(msg) {
			text := msg.Text
			if text == "" && msg.Code > 0 && msg.Code < 128 {
				text = string(msg.Code)
			}
			if text != "" {
				for _, ch := range text {
					a.searchBuf = append(a.searchBuf[:a.searchCursor], append([]rune{ch}, a.searchBuf[a.searchCursor:]...)...)
					a.searchCursor++
				}
				a.runSearch()
			}
		}
	}
	return a, nil
}

// runSearch executes the search query and updates results.
func (a *App) runSearch() {
	if len(a.searchBuf) == 0 {
		a.searchResults = nil
		a.searchIdx = 0
		a.matchedIDs = nil
		a.explorer.ClearFilter()
		return
	}
	query := string(a.searchBuf)
	if a.searchExplorer {
		// Search files in explorer
		a.explorer.Filter(query)
		a.searchResults = nil
		a.matchedIDs = nil
	} else {
		// Search nodes in tree
		filters := search.ParseQuery(query)
		a.searchResults = search.QueryNodes(a.proj, filters)
		a.searchIdx = 0
		a.matchedIDs = make(map[string]bool, len(a.searchResults))
		for _, r := range a.searchResults {
			a.matchedIDs[r.NodeID] = true
		}
	}
}

// jumpToSearchResult moves the tree cursor to the current search result.
func (a *App) jumpToSearchResult() {
	if a.searchIdx < 0 || a.searchIdx >= len(a.searchResults) {
		return
	}
	targetID := a.searchResults[a.searchIdx].NodeID
	a.tree.JumpToNode(targetID)
	a.syncDetailsToTree()
}

// updateCommand handles keys in command mode.
func (a App) updateCommand(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Any key other than "tab" ends a completion-cycling sequence, so the
	// next Tab press starts a fresh match instead of advancing the cycle.
	if msg.String() != "tab" {
		a.cmdCompletionActive = false
	}
	switch msg.String() {
	case "enter":
		cmd := string(a.cmdBuf)
		if cmd != "" {
			a.cmdHistory = append(a.cmdHistory, cmd)
		}
		a.cmdBuf = nil
		a.cmdCursor = 0
		a.cmdHistIdx = -1
		a.mode = ModeNormal
		return a.executeCommand(cmd)
	case "tab":
		a.completeCommand()
	case "up":
		if len(a.cmdHistory) > 0 {
			if a.cmdHistIdx < 0 {
				a.cmdHistIdx = len(a.cmdHistory) - 1
			} else if a.cmdHistIdx > 0 {
				a.cmdHistIdx--
			}
			a.cmdBuf = []rune(a.cmdHistory[a.cmdHistIdx])
			a.cmdCursor = len(a.cmdBuf)
		}
	case "down":
		if a.cmdHistIdx >= 0 {
			if a.cmdHistIdx < len(a.cmdHistory)-1 {
				a.cmdHistIdx++
				a.cmdBuf = []rune(a.cmdHistory[a.cmdHistIdx])
				a.cmdCursor = len(a.cmdBuf)
			} else {
				a.cmdHistIdx = -1
				a.cmdBuf = nil
				a.cmdCursor = 0
			}
		}
	case "left":
		if a.cmdCursor > 0 {
			a.cmdCursor--
		}
	case "right":
		if a.cmdCursor < len(a.cmdBuf) {
			a.cmdCursor++
		}
	case "home", "ctrl+a":
		a.cmdCursor = 0
	case "end", "ctrl+e":
		a.cmdCursor = len(a.cmdBuf)
	case "ctrl+v", "shift+insert":
		return a.pasteFromNativeClipboard()
	case "backspace":
		if a.cmdCursor > 0 {
			a.cmdBuf = append(a.cmdBuf[:a.cmdCursor-1], a.cmdBuf[a.cmdCursor:]...)
			a.cmdCursor--
		}
	case "delete":
		if a.cmdCursor < len(a.cmdBuf) {
			a.cmdBuf = append(a.cmdBuf[:a.cmdCursor], a.cmdBuf[a.cmdCursor+1:]...)
		}
	default:
		if !isControlKey(msg) {
			text := msg.Text
			if text == "" && msg.Code > 0 && msg.Code < 128 {
				text = string(msg.Code)
			}
			if text != "" {
				for _, ch := range text {
					a.cmdBuf = append(a.cmdBuf[:a.cmdCursor], append([]rune{ch}, a.cmdBuf[a.cmdCursor:]...)...)
					a.cmdCursor++
				}
			}
		}
	}
	return a, nil
}

// knownCommands lists all available command names for tab completion.
var knownCommands = []string{"w", "q", "q!", "wq", "export", "theme", "open", "e", "notifications", "notify", "validate", "log-level", "problems", "diagnostics", "registry"}

// knownRegistrySubcommands lists ":registry <sub>" subcommands for tab completion.
var knownRegistrySubcommands = []string{"check", "report"}

// completeCommand attempts to autocomplete the current command buffer. It
// completes the command name itself (first word), and additionally knows
// how to complete ":registry"'s subcommand and, for ":registry check",
// subject names fetched from the registry — reusing the same
// longest-common-prefix completion logic (see completeWord) throughout.
func (a *App) completeCommand() {
	input := string(a.cmdBuf)
	fields := strings.Fields(input)
	endsWithSpace := strings.HasSuffix(input, " ")

	if len(fields) == 0 {
		return
	}

	// First word, not yet followed by a space: completing the command name.
	if len(fields) == 1 && !endsWithSpace {
		a.completeWord(input, knownCommands)
		return
	}

	if fields[0] != "registry" {
		return
	}
	switch {
	case (len(fields) == 1 && endsWithSpace) || (len(fields) == 2 && !endsWithSpace):
		// ":registry " or ":registry <partial>" — complete the subcommand.
		a.completeWord(input, knownRegistrySubcommands)
	case fields[1] == "check" && ((len(fields) == 2 && endsWithSpace) || (len(fields) == 3 && !endsWithSpace)):
		// ":registry check " or ":registry check <partial>" — complete a subject name.
		a.completeWord(input, a.explorer.RegistrySubjects())
	}
}

// completeWord completes the final whitespace-delimited word of input
// against candidates. A single match completes it fully. Multiple matches
// fill in their longest common prefix on the first Tab press (like
// shell/Vim completion) and are remembered on the App so that repeated Tab
// presses cycle through each candidate in turn; renderCommandBar shows the
// candidate list with the active one highlighted so the user can see what
// they're cycling through.
func (a *App) completeWord(input string, candidates []string) {
	// Continuing a cycle started by a previous Tab press on this same word.
	if a.cmdCompletionActive && len(a.cmdCompletions) > 0 {
		a.cmdCompletionIdx++
		if a.cmdCompletionIdx >= len(a.cmdCompletions) {
			a.cmdCompletionIdx = 0
		}
		a.cmdBuf = []rune(a.cmdCompletionPrefix + a.cmdCompletions[a.cmdCompletionIdx])
		a.cmdCursor = len(a.cmdBuf)
		return
	}

	prefix := ""
	partial := input
	if idx := strings.LastIndex(input, " "); idx >= 0 {
		prefix = input[:idx+1]
		partial = input[idx+1:]
	}
	var matches []string
	for _, c := range candidates {
		if strings.HasPrefix(c, partial) {
			matches = append(matches, c)
		}
	}
	if len(matches) == 0 {
		return
	}
	sort.Strings(matches)
	if len(matches) == 1 {
		a.cmdBuf = []rune(prefix + matches[0])
		a.cmdCursor = len(a.cmdBuf)
		a.cmdCompletionActive = false
		a.cmdCompletions = nil
		return
	}

	// Multiple matches: fill in the longest common prefix immediately, and
	// remember the full set so subsequent Tab presses cycle through them.
	common := matches[0]
	for _, m := range matches[1:] {
		for !strings.HasPrefix(m, common) {
			common = common[:len(common)-1]
		}
	}
	a.cmdCompletions = matches
	a.cmdCompletionPrefix = prefix
	a.cmdCompletionActive = true
	if len(common) > len(partial) {
		// Nothing selected yet — just the common prefix is shown; the next
		// Tab press starts cycling at the first candidate.
		a.cmdCompletionIdx = -1
		a.cmdBuf = []rune(prefix + common)
	} else {
		a.cmdCompletionIdx = 0
		a.cmdBuf = []rune(prefix + matches[0])
	}
	a.cmdCursor = len(a.cmdBuf)
}

// executeCommand parses and executes a command string.
func (a App) executeCommand(raw string) (tea.Model, tea.Cmd) {
	raw = strings.TrimSpace(raw)
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return a, nil
	}
	cmd := parts[0]
	args := parts[1:]

	switch cmd {
	case "w":
		if len(args) > 0 {
			return a.doSaveAs(args[0])
		}
		return a.doSave()
	case "q":
		if a.dirty {
			a.confirm = NewConfirm("Unsaved changes. Quit anyway?", a.theme)
			a.overlay = OverlayConfirm
			a.overlayAction = ActionQuit
			return a, nil
		}
		return a.quit()
	case "q!":
		return a.quit()
	case "wq":
		a2, _ := a.doSave()
		app := a2.(App)
		return app.quit()
	case "export":
		if len(args) >= 2 {
			// :export <type> <file>
			return a.doSaveAs(args[1])
		} else if len(args) == 1 {
			return a.doSaveAs(args[0])
		}
		return a.doSave()
	case "theme":
		if len(args) == 0 {
			names := a.themeReg.Names()
			a.picker = NewPicker("Select theme", names, a.theme)
			a.overlay = OverlayPicker
			a.overlayAction = ActionSelectTheme
			return a, nil
		}
		name := args[0]
		t := a.themeReg.Get(name)
		if t == nil {
			a.flashError("Unknown theme: " + name)
			return a, nil
		}
		a.applyTheme(t)
		a.flashInfo("Theme: " + t.Name)
		return a, nil
	case "open", "e":
		if len(args) == 0 {
			a.flashError("Usage: :open <file.avsc>")
			return a, nil
		}
		if a.dirty {
			a.pendingFile = args[0]
			a.confirm = NewConfirm("Unsaved changes. Load anyway?", a.theme)
			a.overlay = OverlayConfirm
			a.overlayAction = ActionLoad
			return a, nil
		}
		return a.openFile(args[0])
	case "notifications", "notify":
		a.overlay = OverlayNotify
		a.notifyScrl = 0
		return a, nil
	case "validate":
		a.runValidation()
		errs := validation.ErrorCount(a.validationIssues)
		warns := validation.WarningCount(a.validationIssues)
		if errs == 0 && warns == 0 {
			a.flashInfo("Schema valid " + a.theme.Sym.Success)
		} else {
			a.flashWarn(fmt.Sprintf("Validation: %d errors, %d warnings", errs, warns))
		}
		return a, nil
	case "log-level":
		if len(args) == 0 {
			levels := []string{"debug", "info", "warn", "error"}
			a.picker = NewPicker("Select log level", levels, a.theme)
			a.overlay = OverlayPicker
			a.overlayAction = ActionSetLogLevel
			return a, nil
		}
		lvl := logging.ParseLevel(args[0])
		logging.SetLevel(lvl)
		a.flashInfo("Log level set to: " + logging.LevelName(lvl))
		return a, nil
	case "problems", "diagnostics":
		a.runValidation()
		if len(a.validationIssues) == 0 {
			a.flashInfo("No problems found " + a.theme.Sym.Success)
			return a, nil
		}
		items, pmap := a.buildProblemsPickerItems()
		a.problemsMap = pmap
		a.picker = NewPicker("Problems", items, a.theme)
		a.overlay = OverlayPicker
		a.overlayAction = ActionShowProblems
		return a, nil
	case "registry":
		return a.executeRegistryCommand(args)
	default:
		a.flashError("Unknown command: :" + raw)
	}
	return a, nil
}

// doSave writes the projection to the file path.
func (a App) doSave() (tea.Model, tea.Cmd) {
	if a.filePath == "" {
		a.flashError("No file — use :w <path>")
		return a, nil
	}
	if err := io.ExportAvroToFile(a.proj, a.filePath); err != nil {
		a.flashError("Save failed: "+err.Error(), "path", a.filePath, "error", err)
		return a, nil
	}
	a.dirty = false
	a.explorer.Refresh()
	a.flashInfo("Saved " + a.filePath)
	return a, nil
}

// doSaveAs writes the projection to a specific path.
func (a App) doSaveAs(path string) (tea.Model, tea.Cmd) {
	// Resolve relative paths against explorer's current directory
	if !filepath.IsAbs(path) {
		path = filepath.Join(a.explorer.RootDir(), path)
	}
	if err := io.ExportAvroToFile(a.proj, path); err != nil {
		a.flashError("Save failed: "+err.Error(), "path", path, "error", err)
		return a, nil
	}
	a.filePath = path
	a.dirty = false
	a.statusbar.SetFile(path)
	a.explorer.Refresh()
	a.flashInfo("Saved " + path)
	return a, nil
}

// applyTheme switches the active theme across all UI components and persists the choice.
func (a *App) applyTheme(t *theme.Theme) {
	a.theme = t
	a.tree.theme = t
	a.details.theme = t
	a.explorer.theme = t
	a.statusbar.theme = t
	config.SetTheme(t.Name)
}

// updateTree handles all keys when the tree panel is focused.
func (a App) updateTree(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch a.mode {
	case ModeNormal:
		switch msg.String() {
		// Undo/Redo
		case "u":
			if a.history.CanUndo() {
				_ = a.history.Undo()
				a.markDirty()
				a.tree.rebuildLines()
				a.syncDetailsToTree()
			}
			return a, nil

		// Actions
		case "a":
			return a.actionAddField()
		case "d":
			return a.actionDelete()
		case "c":
			return a.actionCopy()
		case "m":
			return a.actionMove()
		case "y":
			a.pendingKey = 'y'
			return a, nil

		// Enter focuses the details pane
		case "enter":
			a.focus = PanelDetails
			return a, nil

		// Tree navigation
		default:
			a.tree = a.tree.HandleKey(msg)
			a.syncDetailsToTree()
		}
	}
	return a, nil
}

// updateExplorer handles all keys when the explorer panel is focused.
func (a App) updateExplorer(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		action := a.explorer.EnterSelected()
		switch action.Kind {
		case ExplorerActionOpenFile:
			path := action.Path
			if a.dirty {
				a.pendingFile = path
				a.confirm = NewConfirm("Unsaved changes. Load anyway?", a.theme)
				a.overlay = OverlayConfirm
				a.overlayAction = ActionLoad
				return a, nil
			}
			return a.openFile(path)
		case ExplorerActionFetchSchema:
			return a.beginRegistryFetch(action.Subject, action.Version)
		}
	default:
		a.explorer = a.explorer.HandleKey(msg)
	}
	// l/space (and enter on an unfetched subject) may have queued an async
	// version fetch via toggleExpand; poll for it since those paths don't
	// otherwise return actionable data to App.
	if subject, ok := a.explorer.TakePendingFetch(); ok {
		return a, a.fetchRegistryVersionsCmd(subject)
	}
	return a, nil
}

// openFile loads a new schema file and resets the editor state.
func (a App) openFile(path string) (tea.Model, tea.Cmd) {
	proj, _, err := io.LoadAvroFromFile(path)
	if err != nil {
		a.flashError("Open failed: "+err.Error(), "path", path, "error", err)
		return a, nil
	}
	a.proj = proj
	a.filePath = path
	a.dirty = false
	a.history = command.NewHistory(500)
	a.tree = NewTreeModel(proj, a.theme)
	a.details = NewDetailsModel(proj, a.theme)
	a.statusbar.SetFile(path)
	a.focus = PanelTree
	a.updateLayout()
	a.runValidation()
	a.flashInfo("Opened: " + filepath.Base(path))
	return a, nil
}

// updateDetails handles all keys when the details panel is focused.
func (a App) updateDetails(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch a.mode {
	case ModeNormal:
		switch msg.String() {
		case "esc":
			a.focus = PanelTree
			return a, nil
		case "enter", "e":
			return a.beginDetailEdit()
		case "a":
			return a.actionAddCustomAttr()
		case "d":
			return a.actionDeleteAttr()
		case "R":
			if a.details.IsCustomAttr() {
				if a.details.BeginEditKey() {
					a.mode = ModeEdit
				}
			}
			return a, nil
		case "y":
			a.pendingKey = 'y'
			return a, nil
		default:
			a.details.HandleKey(msg)
		}

	case ModeEdit:
		if a.details.Editing() {
			switch msg.String() {
			case "enter":
				return a.commitDetailEdit()
			case "shift+enter", "alt+enter", "ctrl+j":
				// shift+enter requires terminal support for the Kitty/CSI-u
				// keyboard protocol and is not recognized by many terminals
				// and multiplexers (it arrives identical to plain "enter").
				// alt+enter (meta-sends-escape) and ctrl+j (raw line feed)
				// are recognized by virtually all terminals, so they're
				// offered as reliable fallbacks for inserting a newline.
				attr := a.details.CursorAttr()
				if attr != nil && attr.Kind == AttrMultiline {
					a.details.InsertNewline()
				}
				return a, nil
			case "ctrl+v", "shift+insert":
				return a.pasteFromNativeClipboard()
			case "esc":
				a.details.CancelEdit()
				a.mode = ModeNormal
				return a, nil
			default:
				a.details.HandleKey(msg)
			}
		} else {
			switch msg.String() {
			case "enter", "e":
				return a.beginDetailEdit()
			default:
				a.details.HandleKey(msg)
			}
		}
	}
	return a, nil
}

// beginDetailEdit starts editing the current detail attribute (or opens a picker for select fields).
func (a App) beginDetailEdit() (tea.Model, tea.Cmd) {
	attr := a.details.CursorAttr()
	if attr == nil {
		return a, nil
	}
	switch attr.Kind {
	case AttrSelect:
		if attr.NativeKey == "__type__" {
			a.picker = a.typeCategoryPicker(attr.Key)
			a.overlayAction = ActionReplace
		} else {
			a.picker = NewPicker(attr.Key, attr.Options, a.theme)
			a.overlayAction = ActionSelectAttr
		}
		a.overlay = OverlayPicker
	case AttrReadonly:
		// can't edit
	case AttrListAdd:
		if a.details.BeginEdit() {
			a.mode = ModeEdit
		}
	default:
		if a.details.BeginEdit() {
			a.mode = ModeEdit
		}
	}
	return a, nil
}

// commitDetailEdit finalises the inline edit and executes the appropriate command.
func (a App) commitDetailEdit() (tea.Model, tea.Cmd) {
	wasEditingKey := a.details.editingKey
	attr, newVal, changed := a.details.CommitEdit()
	a.details.editingKey = false
	a.mode = ModeNormal
	if attr == nil || !changed {
		return a, nil
	}

	node := a.details.node
	if node == nil {
		return a, nil
	}

	var cmd *command.Command
	var err error

	isNamedKind := node.Kind == schema.KindRecord || node.Kind == schema.KindEnum || node.Kind == schema.KindFixed
	oldValue := a.details.oldValue

	if wasEditingKey {
		cmd, err = command.RenameAttribute(a.proj, command.RenameAttributeParams{
			NodeID: node.ID,
			OldKey: a.details.oldValue,
			NewKey: newVal,
		})
	} else {
		switch attr.Kind {
		case AttrListItem:
			if isNamedKind && attr.ListKey == "aliases" {
				// Rename alias and propagate to references
				cmd, err = command.RenameAlias(a.proj, command.RenameAliasParams{
					NodeID:   node.ID,
					OldAlias: oldValue,
					NewAlias: newVal,
				})
			} else {
				m := node.NativeMap()
				oldList, _ := m[attr.ListKey].([]any)
				newList := make([]any, len(oldList))
				copy(newList, oldList)
				if attr.ListIndex < len(newList) {
					newList[attr.ListIndex] = newVal
				}
				cmd, err = command.UpdateAttribute(a.proj, command.UpdateAttributeParams{
					NodeID: node.ID, Scope: "native", Key: attr.ListKey, NewValue: newList,
				})
			}

		case AttrListAdd:
			if newVal == "" {
				return a, nil
			}
			m := node.NativeMap()
			var oldList []any
			if m != nil {
				oldList, _ = m[attr.ListKey].([]any)
			}
			newList := make([]any, len(oldList)+1)
			copy(newList, oldList)
			newList[len(oldList)] = newVal
			cmd, err = command.UpdateAttribute(a.proj, command.UpdateAttributeParams{
				NodeID: node.ID, Scope: "native", Key: attr.ListKey, NewValue: newList,
			})

		default:
			if isNamedKind && attr.NativeKey == "name" {
				// Rename type and propagate to all references
				cmd, err = command.RenameNamedType(a.proj, command.RenameNamedTypeParams{
					NodeID:  node.ID,
					OldName: oldValue,
					NewName: newVal,
				})
			} else {
				cmd, err = command.UpdateAttribute(a.proj, command.UpdateAttributeParams{
					NodeID: node.ID, Scope: "native", Key: attr.NativeKey, NewValue: newVal,
				})
			}
		}
	}

	if err != nil || cmd == nil {
		if err != nil {
			a.flashError("Edit failed: "+err.Error(), "node", node.ID, "error", err)
		}
		return a, nil
	}
	_ = a.history.Execute(cmd)
	a.markDirty()
	a.details.rebuildAttrs()
	a.tree.rebuildLines()
	return a, nil
}

// updateOverlay handles keys when an overlay is active.
func (a App) updateOverlay(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch a.overlay {
	case OverlayPicker:
		sel, done := a.picker.HandleKey(msg)
		if done {
			a.overlay = OverlayNone
			if sel != "" {
				return a.handlePickerResult(sel)
			}
		}
	case OverlayConfirm:
		confirmed, done := a.confirm.HandleKey(msg)
		if done {
			a.overlay = OverlayNone
			if confirmed {
				return a.handleConfirmResult()
			}
		}
	case OverlayHelp:
		switch msg.String() {
		case "esc", "?", "q":
			a.overlay = OverlayNone
			a.helpScroll = 0
		case "j", "down":
			a.helpScroll++
		case "k", "up":
			if a.helpScroll > 0 {
				a.helpScroll--
			}
		}
	case OverlayNotify:
		switch msg.String() {
		case "esc", "q":
			a.overlay = OverlayNone
			a.notifyScrl = 0
		case "j", "down":
			a.notifyScrl++
		case "k", "up":
			if a.notifyScrl > 0 {
				a.notifyScrl--
			}
		}
	case OverlayRegistryReport:
		switch msg.String() {
		case "esc", "q":
			a.overlay = OverlayNone
			a.registryReportScrl = 0
		case "j", "down":
			a.registryReportScrl++
		case "k", "up":
			if a.registryReportScrl > 0 {
				a.registryReportScrl--
			}
		case "s":
			a.saveRegistryReport()
		}
	}
	return a, nil
}

// typeCategoryPicker creates a type picker with categorized options.
func (a App) typeCategoryPicker(title string) Picker {
	names, aliases := a.proj.NamedTypesAndAliases()
	cats := []PickerCategory{
		{Label: "Primitives", Items: schema.PrimitiveTypes},
		{Label: "Complex", Items: schema.ComplexTypes},
	}
	if len(names) > 0 {
		cats = append(cats, PickerCategory{Label: "Named Types", Items: names})
	}
	if len(aliases) > 0 {
		cats = append(cats, PickerCategory{Label: "Aliases", Items: aliases})
	}
	return NewCategoryPicker(title, cats, a.theme)
}

// actionAddField initiates adding a new field/branch to the selected node.
func (a App) actionAddField() (tea.Model, tea.Cmd) {
	sel := a.tree.SelectedNode()
	if sel == nil {
		return a, nil
	}

	// Determine target: add as sibling (to parent) unless selected is a record/union
	target := sel
	if sel.Kind == schema.KindField || sel.Kind == schema.KindPrimitive || sel.Kind == schema.KindNamed {
		parent := a.proj.GetParent(sel)
		if parent != nil {
			target = parent
		}
	}

	// What can be added?
	switch target.Kind {
	case schema.KindRecord:
		// Add a new field with type picker
		a.picker = a.typeCategoryPicker("Add field — select type")
		a.overlay = OverlayPicker
		a.overlayAction = ActionAddField
	case schema.KindUnion:
		// Add branch type
		a.picker = a.typeCategoryPicker("Add union branch — select type")
		a.overlay = OverlayPicker
		a.overlayAction = ActionAddField
	default:
		// Can't add to this node type directly
	}
	return a, nil
}

// actionDelete initiates node deletion with confirmation.
func (a App) actionDelete() (tea.Model, tea.Cmd) {
	sel := a.tree.SelectedNode()
	if sel == nil || sel.ID == a.proj.RootID {
		return a, nil
	}
	if a.proj.IsInSingleSlot(sel) {
		return a, nil
	}
	cmd, err := command.RemoveNode(a.proj, command.RemoveNodeParams{NodeID: sel.ID})
	if err != nil {
		a.flashError("Delete failed: "+err.Error(), "node", sel.ID, "error", err)
		return a, nil
	}
	_ = a.history.Execute(cmd)
	a.markDirty()
	a.tree.rebuildLines()
	a.syncDetailsToTree()
	return a, nil
}

// actionAddCustomAttr adds a new custom attribute to the current node via command.
func (a App) actionAddCustomAttr() (tea.Model, tea.Cmd) {
	node := a.details.node
	if node == nil {
		return a, nil
	}

	m := node.NativeMap()
	if m == nil {
		return a, nil
	}

	// Generate a unique key name
	key := "x-custom"
	if _, exists := m[key]; exists {
		for i := 1; ; i++ {
			candidate := fmt.Sprintf("x-custom-%d", i)
			if _, exists := m[candidate]; !exists {
				key = candidate
				break
			}
		}
	}

	cmd, err := command.UpdateAttribute(a.proj, command.UpdateAttributeParams{
		NodeID:   node.ID,
		Scope:    "native",
		Key:      key,
		NewValue: "",
	})
	if err != nil {
		a.flashError("Add attribute failed: "+err.Error(), "node", node.ID, "key", key, "error", err)
		return a, nil
	}
	_ = a.history.Execute(cmd)
	a.markDirty()
	a.details.rebuildAttrs()
	// Move cursor to the new attribute (last one)
	a.details.cursor = len(a.details.attrs) - 1
	return a, nil
}

// actionDeleteAttr deletes the attribute under cursor (custom attr or list item).
func (a App) actionDeleteAttr() (tea.Model, tea.Cmd) {
	node := a.details.node
	if node == nil {
		return a, nil
	}

	attr := a.details.CursorAttr()
	if attr == nil {
		return a, nil
	}

	var cmd *command.Command
	var err error

	switch {
	case attr.Kind == AttrListItem:
		m := node.NativeMap()
		if m == nil {
			return a, nil
		}
		oldList, _ := m[attr.ListKey].([]any)
		if attr.ListIndex >= len(oldList) {
			return a, nil
		}
		newList := make([]any, 0, len(oldList)-1)
		for i, v := range oldList {
			if i != attr.ListIndex {
				newList = append(newList, v)
			}
		}
		cmd, err = command.UpdateAttribute(a.proj, command.UpdateAttributeParams{
			NodeID: node.ID, Scope: "native", Key: attr.ListKey, NewValue: newList,
		})

	case a.details.IsCustomAttr():
		key := attr.NativeKey
		if key == "" {
			return a, nil
		}
		cmd, err = command.DeleteAttribute(a.proj, command.DeleteAttributeParams{
			NodeID: node.ID, Scope: "native", Key: key,
		})

	default:
		return a, nil
	}

	if err != nil || cmd == nil {
		if err != nil {
			a.flashError("Delete failed: " + err.Error())
		}
		return a, nil
	}
	_ = a.history.Execute(cmd)
	a.markDirty()
	a.details.rebuildAttrs()
	if a.details.cursor >= len(a.details.attrs) && a.details.cursor > 0 {
		a.details.cursor--
	}
	return a, nil
}

// actionCopy duplicates the selected node as a sibling.
func (a App) actionCopy() (tea.Model, tea.Cmd) {
	sel := a.tree.SelectedNode()
	if sel == nil || sel.ID == a.proj.RootID {
		return a, nil
	}
	if a.proj.IsInSingleSlot(sel) {
		return a, nil
	}

	parent := a.proj.GetParent(sel)
	if parent == nil {
		return a, nil
	}

	cmd, err := command.CopyNode(a.proj, command.CopyNodeParams{
		SourceID: sel.ID,
		TargetID: parent.ID,
		Index:    -1,
	})
	if err != nil {
		a.flashError("Copy failed: "+err.Error(), "source", sel.ID, "target", parent.ID, "error", err)
		return a, nil
	}
	_ = a.history.Execute(cmd)
	a.markDirty()
	a.tree.rebuildLines()
	a.syncDetailsToTree()
	return a, nil
}

// handleYankKey resolves a pending "y" + motion sequence. Behavior depends
// on which panel is focused: in the tree pane "yy" yanks the selected
// node's name; in the details pane "yp"/"yy"/"ya" yank the current
// attribute's name/value/"name: value". Any other combination (including
// "y" pressed in the explorer) is a silent no-op, matching vim's handling
// of an unrecognized operator+motion.
func (a App) handleYankKey(pending rune, key string) (tea.Model, tea.Cmd) {
	if pending != 'y' {
		return a, nil
	}
	switch a.focus {
	case PanelTree:
		if key != "y" {
			return a, nil
		}
		node := a.tree.SelectedNode()
		if node == nil {
			return a, nil
		}
		return a.yank(node.Name(), "field name")
	case PanelDetails:
		attr := a.details.CursorAttr()
		if attr == nil {
			return a, nil
		}
		switch key {
		case "p":
			return a.yank(attr.Key, "property name")
		case "y":
			return a.yank(attr.Value, "property value")
		case "a":
			return a.yank(attr.Key+": "+attr.Value, "property")
		}
	}
	return a, nil
}

// yank copies text to the clipboard (OSC52 + native tool, see
// copyToClipboard) and flashes a status message naming what was copied.
func (a App) yank(text, label string) (tea.Model, tea.Cmd) {
	if text == "" {
		a.flashError("nothing to yank: " + label + " is empty")
		return a, nil
	}
	a.flashInfo("yanked " + label)
	return a, copyToClipboard(text)
}

// copyToClipboard writes text to the clipboard using both OSC52 (via the
// terminal itself, works over SSH with no subprocess) and a best-effort
// native tool. Either landing is a win and there's no real cost when one
// doesn't, so both are always attempted.
func copyToClipboard(text string) tea.Cmd {
	return tea.Batch(
		tea.SetClipboard(text),
		func() tea.Msg {
			_ = clipboard.WriteNative(text)
			return nil
		},
	)
}

// pasteFromNativeClipboard is the explicit "paste" fallback (ctrl+v or
// shift+insert) for terminals/sessions where bracketed-paste doesn't reach
// the app (e.g. no tea.PasteMsg fires). Silently no-ops if no native
// clipboard tool is available — this is a best-effort convenience, not the
// primary path (tea.PasteMsg is).
func (a App) pasteFromNativeClipboard() (tea.Model, tea.Cmd) {
	text, err := clipboard.ReadNative()
	if err != nil {
		return a, nil
	}
	a.pasteText(text)
	return a, nil
}

// pasteText inserts s into whichever text input is currently active: the
// details-pane inline editor, the search bar, or the command bar. A no-op
// elsewhere (e.g. normal-mode navigation).
func (a *App) pasteText(s string) {
	if s == "" {
		return
	}
	switch {
	case a.mode == ModeEdit && a.details.Editing():
		a.details.PasteText(s)
	case a.mode == ModeSearch:
		a.searchBuf, a.searchCursor = insertRunesAt(a.searchBuf, a.searchCursor, s)
		a.runSearch()
	case a.mode == ModeCommand:
		a.cmdBuf, a.cmdCursor = insertRunesAt(a.cmdBuf, a.cmdCursor, s)
	}
}

// insertRunesAt inserts text into buf at the given rune cursor position,
// returning the updated buffer and the cursor position after the inserted
// text.
func insertRunesAt(buf []rune, cursor int, text string) ([]rune, int) {
	runes := []rune(text)
	out := make([]rune, 0, len(buf)+len(runes))
	out = append(out, buf[:cursor]...)
	out = append(out, runes...)
	out = append(out, buf[cursor:]...)
	return out, cursor + len(runes)
}

// actionMove opens a picker to choose a target record for moving the selected field.
func (a App) actionMove() (tea.Model, tea.Cmd) {
	sel := a.tree.SelectedNode()
	if sel == nil || sel.ID == a.proj.RootID {
		return a, nil
	}
	if a.proj.IsInSingleSlot(sel) {
		return a, nil
	}

	// Collect all records as valid move targets (excluding current parent)
	currentParent := sel.ParentID
	targets := make(map[string]string) // display name → node ID
	var items []string

	for id, node := range a.proj.Nodes {
		if node.Kind != schema.KindRecord {
			continue
		}
		if id == currentParent {
			continue
		}
		// Build display name from path
		name := node.Name()
		if ns := node.Namespace(); ns != "" {
			name = ns + "." + name
		}
		targets[name] = id
		items = append(items, name)
	}

	if len(items) == 0 {
		return a, nil
	}

	a.moveTargets = targets
	a.picker = NewPicker("Move to record", items, a.theme)
	a.overlay = OverlayPicker
	a.overlayAction = ActionMove
	return a, nil
}

// actionReplaceType initiates type replacement on the selected node.
func (a App) actionReplaceType() (tea.Model, tea.Cmd) {
	sel := a.tree.SelectedNode()
	if sel == nil {
		return a, nil
	}

	// Validate that the selected node or its parent is a type-bearing node
	if sel.Kind != schema.KindField && sel.Kind != schema.KindArray && sel.Kind != schema.KindMap {
		parent := a.proj.GetParent(sel)
		if parent == nil || (parent.Kind != schema.KindField && parent.Kind != schema.KindArray && parent.Kind != schema.KindMap) {
			return a, nil
		}
	}

	a.picker = a.typeCategoryPicker("Replace type")
	a.overlay = OverlayPicker
	a.overlayAction = ActionReplace
	return a, nil
}

// handlePickerResult processes the selection from the picker overlay.
func (a App) handlePickerResult(sel string) (tea.Model, tea.Cmd) {
	switch a.overlayAction {
	case ActionAddField:
		node := a.tree.SelectedNode()
		if node == nil {
			return a, nil
		}
		target := node
		if node.Kind == schema.KindField || node.Kind == schema.KindPrimitive || node.Kind == schema.KindNamed {
			if parent := a.proj.GetParent(node); parent != nil {
				target = parent
			}
		}
		return a.doAddField(target, sel)

	case ActionReplace:
		node := a.tree.SelectedNode()
		if node == nil {
			return a, nil
		}
		target := node
		if node.Kind != schema.KindField && node.Kind != schema.KindArray && node.Kind != schema.KindMap {
			if parent := a.proj.GetParent(node); parent != nil {
				target = parent
			}
		}
		return a.doReplaceType(target, sel)

	case ActionSelectAttr:
		attr := a.details.CursorAttr()
		if attr == nil || a.details.node == nil {
			return a, nil
		}
		node := a.details.node

		// Special: logicalType on a primitive in string form needs native conversion
		if attr.NativeKey == "logicalType" && node.Kind == schema.KindPrimitive && node.NativeMap() == nil {
			baseType := node.NativeString()
			oldNative := node.Attrs.Native
			var newNative any
			if sel == "(none)" {
				newNative = baseType
			} else {
				newNative = map[string]any{"type": baseType, "logicalType": sel}
			}
			cmd := &command.Command{
				DoFn:   func() error { node.Attrs.Native = newNative; return nil },
				UndoFn: func() error { node.Attrs.Native = oldNative; return nil },
				Desc:   fmt.Sprintf("Set logicalType=%s", sel),
			}
			_ = a.history.Execute(cmd)
			a.markDirty()
			a.details.rebuildAttrs()
			return a, nil
		}

		var newVal any = sel
		if sel == "(none)" {
			newVal = nil
		}
		cmd, err := command.UpdateAttribute(a.proj, command.UpdateAttributeParams{
			NodeID: node.ID, Scope: "native", Key: attr.NativeKey, NewValue: newVal,
		})
		if err == nil {
			_ = a.history.Execute(cmd)
			a.markDirty()
			a.details.rebuildAttrs()
		}
		return a, nil

	case ActionMove:
		targetID, ok := a.moveTargets[sel]
		if !ok {
			return a, nil
		}
		selNode := a.tree.SelectedNode()
		if selNode == nil {
			return a, nil
		}
		cmd, err := command.MoveNode(a.proj, command.MoveNodeParams{
			NodeID:   selNode.ID,
			TargetID: targetID,
			Index:    -1,
		})
		if err != nil {
			a.flashError("Move failed: "+err.Error(), "node", selNode.ID, "target", targetID, "error", err)
			return a, nil
		}
		_ = a.history.Execute(cmd)
		a.markDirty()
		a.tree.rebuildLines()
		a.syncDetailsToTree()
		return a, nil

	case ActionSelectTheme:
		t := a.themeReg.Get(sel)
		if t == nil {
			return a, nil
		}
		a.applyTheme(t)
		a.flashInfo("Theme: " + t.Name)
		return a, nil

	case ActionSetLogLevel:
		lvl := logging.ParseLevel(sel)
		logging.SetLevel(lvl)
		a.flashInfo("Log level set to: " + logging.LevelName(lvl))
		return a, nil

	case ActionShowProblems:
		nodeID, ok := a.problemsMap[sel]
		if !ok || nodeID == "" {
			return a, nil
		}
		a.tree.JumpToNode(nodeID)
		a.syncDetailsToTree()
		a.focus = PanelTree
		return a, nil
	}
	return a, nil
}

// doAddField creates and executes the add-field command.
func (a App) doAddField(target *projection.Node, typeName string) (tea.Model, tea.Cmd) {
	switch target.Kind {
	case schema.KindRecord:
		// Create a new field spec
		fieldSpec := schema.NewFieldSpec("new_field", typeName)
		fieldRaw := map[string]any(fieldSpec)

		// Build a mini projection from the field spec
		tempProj := projection.New()
		tempNode := projection.NewNodeDetached(schema.KindField, fieldRaw, []any{})
		tempProj.Nodes[tempNode.ID] = tempNode

		// Build type child (with sub-children for array/map)
		typeNode, typeNodes := buildTypeSubtree(typeName)
		typeNode.ParentID = tempNode.ID
		for _, n := range typeNodes {
			tempProj.Nodes[n.ID] = n
		}
		tempNode.Children = append(tempNode.Children, typeNode.ID)

		allNodes := append([]*projection.Node{tempNode}, typeNodes...)
		cloneResult := &projection.CloneResult{
			Root:  tempNode,
			Nodes: allNodes,
		}

		cmd, err := command.CreateNode(a.proj, command.CreateNodeParams{
			NewSubtree: cloneResult,
			TargetID:   target.ID,
			Index:      -1,
		})
		if err != nil {
			a.flashError("Add field failed: "+err.Error(), "target", target.ID, "type", typeName, "error", err)
			return a, nil
		}
		_ = a.history.Execute(cmd)
		a.markDirty()
		a.tree.rebuildLines()
		a.syncDetailsToTree()

	case schema.KindUnion:
		// Add a branch to the union
		branchNode, branchNodes := buildTypeSubtree(typeName)

		cloneResult := &projection.CloneResult{
			Root:  branchNode,
			Nodes: branchNodes,
		}

		cmd, err := command.CreateNode(a.proj, command.CreateNodeParams{
			NewSubtree: cloneResult,
			TargetID:   target.ID,
			Index:      -1,
		})
		if err != nil {
			a.flashError("Add branch failed: "+err.Error(), "target", target.ID, "type", typeName, "error", err)
			return a, nil
		}
		_ = a.history.Execute(cmd)
		a.markDirty()
		a.tree.rebuildLines()
		a.syncDetailsToTree()
	}
	return a, nil
}

// doReplaceType creates and executes the replace-type command.
func (a App) doReplaceType(target *projection.Node, typeName string) (tea.Model, tea.Cmd) {
	newTypeNode, allNodes := buildTypeSubtree(typeName)

	cloneResult := &projection.CloneResult{
		Root:  newTypeNode,
		Nodes: allNodes,
	}

	cmd, err := command.ReplaceType(a.proj, command.ReplaceTypeParams{
		ParentID:   target.ID,
		NewSubtree: cloneResult,
	})
	if err != nil {
		a.flashError("Replace type failed: "+err.Error(), "target", target.ID, "type", typeName, "error", err)
		return a, nil
	}
	_ = a.history.Execute(cmd)
	a.markDirty()
	a.tree.rebuildLines()
	a.syncDetailsToTree()
	return a, nil
}

// handleConfirmResult processes a confirmed action.
func (a App) handleConfirmResult() (tea.Model, tea.Cmd) {
	switch a.overlayAction {
	case ActionDelete:
		sel := a.tree.SelectedNode()
		if sel == nil {
			return a, nil
		}
		cmd, err := command.RemoveNode(a.proj, command.RemoveNodeParams{NodeID: sel.ID})
		if err != nil {
			a.flashError("Delete failed: "+err.Error(), "node", sel.ID, "error", err)
			return a, nil
		}
		_ = a.history.Execute(cmd)
		a.markDirty()
		a.tree.rebuildLines()
		a.syncDetailsToTree()
	case ActionLoad:
		if a.pendingFile != "" {
			return a.openFile(a.pendingFile)
		}
	case ActionLoadRegistry:
		if a.pendingRegistrySubject != "" {
			subject, version := a.pendingRegistrySubject, a.pendingRegistryVersion
			a.pendingRegistrySubject = ""
			a.flashInfo(fmt.Sprintf("Registry: fetching %s v%d...", subject, version))
			return a, a.fetchRegistrySchemaCmd(subject, version)
		}
	case ActionQuit:
		return a.quit()
	}
	return a, nil
}

// cycleFocus moves focus to the next panel.
func (a *App) cycleFocus() {
	switch a.focus {
	case PanelTree:
		a.focus = PanelDetails
	case PanelDetails:
		if a.explorer.Visible() {
			a.focus = PanelExplorer
		} else {
			a.focus = PanelTree
		}
	case PanelExplorer:
		a.focus = PanelTree
	}
}

// focusPanel sets focus to a specific panel by number (1=explorer, 2=tree, 3=details).
func (a *App) focusPanel(n int) {
	switch n {
	case 1:
		if !a.explorer.Visible() {
			a.explorer.Toggle()
			a.updateLayout()
		}
		a.focus = PanelExplorer
	case 2:
		a.focus = PanelTree
	case 3:
		a.focus = PanelDetails
	}
}

// syncDetailsToTree updates the details panel based on tree selection.
func (a *App) syncDetailsToTree() {
	if sel := a.tree.SelectedNode(); sel != nil {
		a.details.SetNode(sel)
	}
}

// updateLayout recalculates panel sizes based on terminal dimensions.
func (a *App) updateLayout() {
	// Reserve 1 row for status bar
	contentHeight := a.height - 1
	if contentHeight < 0 {
		contentHeight = 0
	}

	if a.explorer.Visible() {
		// 3-panel: 20% explorer, 30% tree, 50% details
		explorerWidth := a.width / 5
		treeWidth := a.width * 3 / 10
		detailWidth := a.width - explorerWidth - treeWidth
		a.explorer.SetSize(explorerWidth, contentHeight)
		a.tree.SetSize(treeWidth, contentHeight)
		a.details.SetSize(detailWidth, contentHeight)
	} else {
		// 2-panel: 40% tree, 60% details
		treeWidth := a.width * 2 / 5
		detailWidth := a.width - treeWidth
		a.tree.SetSize(treeWidth, contentHeight)
		a.details.SetSize(detailWidth, contentHeight)
	}
	a.statusbar.SetWidth(a.width)
}

// View implements tea.Model.
func (a App) View() tea.View {
	if !a.ready {
		return tea.NewView("Initializing...")
	}

	// Sync search matches to tree
	a.tree.matchedIDs = a.matchedIDs

	// Update status bar state
	a.statusbar.SetMode(a.mode)
	stats := projection.CalculateStats(a.proj)
	// Set node path in status bar: full selected explorer entry name when the
	// explorer is focused (its own panel truncates long names to fit width),
	// otherwise the tree's selected node path.
	if a.focus == PanelExplorer {
		if name := a.explorer.SelectedDisplayName(); name != "" {
			a.statusbar.SetNodePath(name)
		} else {
			a.statusbar.SetNodePath("")
		}
	} else if sel := a.tree.SelectedNode(); sel != nil && len(sel.Path) > 0 {
		a.statusbar.SetNodePath(formatNodePath(sel.Path))
	} else {
		a.statusbar.SetNodePath("")
	}

	statsText := fmt.Sprintf("%d fields, depth %d", stats.FieldCount, stats.MaxDepth)
	if errs := validation.ErrorCount(a.validationIssues); errs > 0 {
		statsText += fmt.Sprintf(" %s%d", a.theme.Sym.Error, errs)
	}
	if warns := validation.WarningCount(a.validationIssues); warns > 0 {
		statsText += fmt.Sprintf(" %s%d", a.theme.Sym.Warning, warns)
	}
	a.statusbar.SetStats(statsText)
	a.statusbar.SetUndo(a.history.UndoCount(), a.history.RedoCount())

	// Render panels with appropriate border styles
	treeView := a.renderPanel(a.tree.View(), PanelTree)
	detailView := a.renderPanel(a.details.View(), PanelDetails)

	// Layout: [explorer |] tree | details (horizontal)
	var content string
	if a.explorer.Visible() {
		explorerView := a.renderPanel(a.explorer.View(), PanelExplorer)
		content = lipgloss.JoinHorizontal(lipgloss.Top, explorerView, treeView, detailView)
	} else {
		content = lipgloss.JoinHorizontal(lipgloss.Top, treeView, detailView)
	}

	// If overlay is active, render it over content
	if a.overlay != OverlayNone {
		content = a.renderOverlay()
	}

	// Bottom bar: either search/command input or standard statusbar
	var bottomBar string
	switch a.mode {
	case ModeSearch:
		bottomBar = a.renderSearchBar()
	case ModeCommand:
		bottomBar = a.renderCommandBar()
	default:
		if a.flash != "" {
			bottomBar = a.renderFlashBar()
		} else {
			bottomBar = a.statusbar.View()
		}
	}

	// Stack: content + bottom bar (vertical)
	full := lipgloss.JoinVertical(lipgloss.Left, content, bottomBar)

	v := tea.NewView(full)
	v.AltScreen = true
	return v
}

// renderPanel wraps a panel view with a titled border (lazygit-style).
func (a App) renderPanel(content string, panel FocusPanel) string {
	// Determine panel number and title (static: 1=Files, 2=Schema, 3=Details)
	var num string
	var title string
	switch panel {
	case PanelExplorer:
		num, title = "1", a.explorer.Title()
	case PanelTree:
		num, title = "2", "Schema"
	case PanelDetails:
		num, title = "3", "Details"
	}

	// Get dimensions
	var w, h int
	switch panel {
	case PanelTree:
		w, h = a.tree.width, a.tree.height
	case PanelDetails:
		w, h = a.details.width, a.details.height
	case PanelExplorer:
		w, h = a.explorer.width, a.explorer.height
	}
	innerW := w - 2
	innerH := h - 2
	if innerW < 1 {
		innerW = 1
	}
	if innerH < 1 {
		innerH = 1
	}

	// Pick border color based on focus
	focused := panel == a.focus
	var borderFg lipgloss.Style
	var titleStyle lipgloss.Style
	if focused {
		borderFg = lipgloss.NewStyle().Foreground(a.theme.Primary)
		titleStyle = lipgloss.NewStyle().Foreground(a.theme.Primary).Bold(true)
	} else {
		borderFg = lipgloss.NewStyle().Foreground(a.theme.Muted)
		titleStyle = lipgloss.NewStyle().Foreground(a.theme.Dim)
	}

	// Build the titled top border: ╭─ N Title ─────╮
	// The title (e.g. a long registry hostname) must never make the border
	// wider than the panel — truncate it to fit, keeping the number prefix
	// visible, otherwise the border overflows and misaligns sibling panels.
	titleText := " " + num + " " + title + " "
	maxTitleWidth := innerW - 2
	if maxTitleWidth < 0 {
		maxTitleWidth = 0
	}
	if lipgloss.Width(titleText) > maxTitleWidth {
		titleText = ansi.Truncate(titleText, maxTitleWidth, "…")
	}
	styledTitle := titleStyle.Render(titleText)
	titleWidth := lipgloss.Width(styledTitle)

	topLeft := borderFg.Render("╭─")
	topRight := borderFg.Render("─╮")
	remainingW := innerW - titleWidth - 2
	if remainingW < 0 {
		remainingW = 0
	}
	topFill := borderFg.Render(strings.Repeat("─", remainingW))
	topBorder := topLeft + styledTitle + topFill + topRight

	// Build bottom border: ╰────────────────╯
	bottomLeft := borderFg.Render("╰")
	bottomRight := borderFg.Render("╯")
	bottomFill := borderFg.Render(strings.Repeat("─", innerW))
	bottomBorder := bottomLeft + bottomFill + bottomRight

	// Render content lines with side borders
	lines := strings.Split(content, "\n")
	var body strings.Builder
	leftBorder := borderFg.Render("│")
	rightBorder := borderFg.Render("│")

	for i := 0; i < innerH; i++ {
		var line string
		if i < len(lines) {
			line = lines[i]
		}
		// Truncate line if it exceeds available width
		lineW := lipgloss.Width(line)
		if lineW > innerW {
			line = ansi.Truncate(line, innerW, "…")
			lineW = lipgloss.Width(line)
		}
		// Pad line to fill width
		if lineW < innerW {
			line += strings.Repeat(" ", innerW-lineW)
		}
		body.WriteString(leftBorder + line + rightBorder)
		if i < innerH-1 {
			body.WriteByte('\n')
		}
	}

	return topBorder + "\n" + body.String() + "\n" + bottomBorder
}

// renderOverlay renders the active overlay centered on the content area.
func (a App) renderOverlay() string {
	var overlayContent string
	switch a.overlay {
	case OverlayPicker:
		overlayContent = a.picker.View()
	case OverlayConfirm:
		overlayContent = a.confirm.View()
	case OverlayHelp:
		helpText := HelpView(a.theme)
		helpLines := strings.Split(helpText, "\n")
		// Limit visible height to terminal - 6 (border + padding)
		maxLines := a.height - 8
		if maxLines < 5 {
			maxLines = 5
		}
		// Clamp scroll
		maxScroll := len(helpLines) - maxLines
		if maxScroll < 0 {
			maxScroll = 0
		}
		if a.helpScroll > maxScroll {
			a.helpScroll = maxScroll
		}
		end := a.helpScroll + maxLines
		if end > len(helpLines) {
			end = len(helpLines)
		}
		visible := helpLines[a.helpScroll:end]
		// Add scroll indicator
		dimStyle := lipgloss.NewStyle().Foreground(a.theme.Muted).Italic(true)
		hint := "  esc/? close  j/k scroll"
		if a.helpScroll > 0 {
			hint += "  " + a.theme.Sym.ScrollUp
		}
		if a.helpScroll < maxScroll {
			hint += "  " + a.theme.Sym.ScrollDown
		}
		visible = append(visible, "", dimStyle.Render(hint))
		overlayContent = strings.Join(visible, "\n")

	case OverlayNotify:
		title := lipgloss.NewStyle().Bold(true).Foreground(a.theme.Primary).Render("Notifications")
		var lines []string
		lines = append(lines, title, "")
		if len(a.notifyLog) == 0 {
			lines = append(lines, "  No notifications yet.")
		} else {
			for i := len(a.notifyLog) - 1; i >= 0; i-- {
				entry := a.notifyLog[i]
				var prefix string
				var clr color.Color
				switch entry.Level {
				case FlashError:
					prefix = a.theme.Sym.Error + " "
					clr = a.theme.Error
				case FlashWarning:
					prefix = a.theme.Sym.Warning + " "
					clr = a.theme.Warning
				default:
					prefix = a.theme.Sym.Success + " "
					clr = a.theme.Success
				}
				line := lipgloss.NewStyle().Foreground(clr).Render(prefix + entry.Message)
				lines = append(lines, line)
			}
		}
		maxLines := a.height - 8
		if maxLines < 5 {
			maxLines = 5
		}
		maxScroll := len(lines) - maxLines
		if maxScroll < 0 {
			maxScroll = 0
		}
		if a.notifyScrl > maxScroll {
			a.notifyScrl = maxScroll
		}
		end := a.notifyScrl + maxLines
		if end > len(lines) {
			end = len(lines)
		}
		visible := lines[a.notifyScrl:end]
		dimStyle := lipgloss.NewStyle().Foreground(a.theme.Muted).Italic(true)
		hint := "  esc close  j/k scroll"
		if a.notifyScrl > 0 {
			hint += "  " + a.theme.Sym.ScrollUp
		}
		if a.notifyScrl < maxScroll {
			hint += "  " + a.theme.Sym.ScrollDown
		}
		visible = append(visible, "", dimStyle.Render(hint))
		overlayContent = strings.Join(visible, "\n")

	case OverlayRegistryReport:
		overlayContent = a.renderRegistryReportOverlay()
	}

	overlayBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(a.theme.Primary).
		Padding(1, 2).
		Render(overlayContent)

	contentH := a.height - 1
	if contentH < 1 {
		contentH = 1
	}
	return lipgloss.Place(a.width, contentH, lipgloss.Center, lipgloss.Center, overlayBox)
}

// renderSearchBar renders the search input bar.
func (a App) renderSearchBar() string {
	label := "/"
	if a.searchExplorer {
		label = "/ files"
	}
	prompt := a.theme.StatusSearch.Render(" " + label + " ")
	input := renderLineInput(a.searchBuf, a.searchCursor, a.theme.StatusFile)
	var info string
	if a.searchExplorer {
		count := a.explorer.FilteredCount()
		if count > 0 {
			info = a.theme.StatusStats.Render(fmt.Sprintf(" [%d/%d]", a.explorer.FilterIdx()+1, count))
		} else if len(a.searchBuf) > 0 {
			info = a.theme.StatusHelp.Render(" no matches")
		}
	} else {
		if len(a.searchResults) > 0 {
			info = a.theme.StatusStats.Render(fmt.Sprintf(" [%d/%d]", a.searchIdx+1, len(a.searchResults)))
		} else if len(a.searchBuf) > 0 {
			info = a.theme.StatusHelp.Render(" no matches")
		}
	}
	bar := prompt + input + info
	return a.theme.StatusBar.Width(a.width).Render(bar)
}

// renderCommandBar renders the command input bar.
func (a App) renderCommandBar() string {
	prompt := a.theme.StatusCommand.Render(" : ")
	input := renderLineInput(a.cmdBuf, a.cmdCursor, a.theme.StatusFile)
	bar := prompt + input
	if a.cmdCompletionActive && len(a.cmdCompletions) > 0 {
		bar += "  " + a.renderCompletionSuggestions()
	}
	// Guard against the suggestion list (or a long command buffer) making
	// the bar wider than the terminal — StatusBar.Width only pads, it does
	// not truncate, so an overlong bar would wrap/overflow the terminal.
	if lipgloss.Width(bar) > a.width {
		bar = ansi.Truncate(bar, a.width, "…")
	}
	return a.theme.StatusBar.Width(a.width).Render(bar)
}

// renderCompletionSuggestions renders the current tab-completion candidate
// list inline (e.g. after ":registry check e<Tab>"), highlighting whichever
// candidate is currently applied to cmdBuf so that cycling through matches
// with repeated Tab presses is visible to the user.
func (a App) renderCompletionSuggestions() string {
	dim := lipgloss.NewStyle().Foreground(a.theme.Dim)
	active := lipgloss.NewStyle().Foreground(a.theme.Primary).Bold(true).Underline(true)
	parts := make([]string, len(a.cmdCompletions))
	for i, c := range a.cmdCompletions {
		if i == a.cmdCompletionIdx {
			parts[i] = active.Render(c)
		} else {
			parts[i] = dim.Render(c)
		}
	}
	return dim.Render("[") + strings.Join(parts, dim.Render(" ")) + dim.Render("]")
}

// renderLineInput renders a rune buffer with a thin line cursor.
func renderLineInput(buf []rune, cursor int, style lipgloss.Style) string {
	before := string(buf[:cursor])
	after := ""
	if cursor < len(buf) {
		after = string(buf[cursor:])
	}
	// Strip padding to avoid extra spaces between segments
	textStyle := style.Padding(0)
	cursorStyle := lipgloss.NewStyle().Bold(true).Foreground(style.GetForeground())
	return textStyle.Render(before) + cursorStyle.Render("│") + textStyle.Render(after)
}

// renderFlashBar renders a temporary message bar with level-based coloring.
func (a App) renderFlashBar() string {
	style := lipgloss.NewStyle().PaddingLeft(1)
	switch a.flashLevel {
	case FlashError:
		style = style.Foreground(a.theme.Error)
	case FlashWarning:
		style = style.Foreground(a.theme.Warning)
	default:
		style = style.Foreground(a.theme.Success)
	}
	msg := style.Render(a.flash)
	return a.theme.StatusBar.Width(a.width).Render(msg)
}

func (a *App) setFlash(msg string, level FlashLevel) {
	a.flash = msg
	a.flashLevel = level
	a.notifyLog = append(a.notifyLog, NotifyEntry{Message: msg, Level: level})
	// Cap at 50 entries
	if len(a.notifyLog) > 50 {
		a.notifyLog = a.notifyLog[len(a.notifyLog)-50:]
	}
}

func (a *App) flashInfo(msg string) {
	a.setFlash(msg, FlashInfo)
	slog.Info(msg)
}

func (a *App) flashWarn(msg string, logCtx ...any) {
	a.setFlash(msg, FlashWarning)
	slog.Warn(msg, logCtx...)
}

func (a *App) flashError(msg string, logCtx ...any) {
	a.setFlash(msg, FlashError)
	slog.Error(msg, logCtx...)
}

// markDirty sets the dirty flag and re-runs advisory validation.
// Central hook point — all mutations should call this instead of setting dirty directly.
// Future module system will extend this with additional post-mutation hooks.
func (a *App) markDirty() {
	a.dirty = true
	a.runValidation()
}

// runValidation executes the advisory validation analyzer against the current projection.
// Results are stored for display in status bar and details pane.
// Never blocks or reverts mutations.
func (a *App) runValidation() {
	if a.validator == nil || a.proj == nil {
		a.validationIssues = nil
		return
	}
	a.validationIssues = a.validator.Analyze(a.proj)
}

// buildProblemsPickerItems formats validation issues for the picker.
// Returns display items (errors first, then warnings) and a map from display string to node ID.
func (a *App) buildProblemsPickerItems() ([]string, map[string]string) {
	pmap := make(map[string]string, len(a.validationIssues))
	var errors, warnings []string

	for _, iss := range a.validationIssues {
		// Get the node name for context
		nodeName := ""
		if node := a.proj.Nodes[iss.NodeID]; node != nil {
			nodeName = nodeLabel(node)
		}

		var line string
		switch iss.Severity {
		case validation.SeverityError:
			line = a.theme.Sym.Error + " " + iss.Message
		default:
			line = a.theme.Sym.Warning + " " + iss.Message
		}
		if nodeName != "" {
			line += " (" + nodeName + ")"
		}

		// Ensure uniqueness (append node ID suffix if duplicate)
		if _, exists := pmap[line]; exists {
			line += " [" + iss.NodeID + "]"
		}
		pmap[line] = iss.NodeID

		switch iss.Severity {
		case validation.SeverityError:
			errors = append(errors, line)
		default:
			warnings = append(warnings, line)
		}
	}

	items := append(errors, warnings...)
	return items, pmap
}

// quit persists history and returns the quit command.
func (a App) quit() (tea.Model, tea.Cmd) {
	config.SaveHistory(config.History{
		Commands: a.cmdHistory,
		Searches: a.searchHistory,
	})
	return a, tea.Quit
}

// formatNodePath renders a node's JSON path for the status bar.
func formatNodePath(path []any) string {
	parts := make([]string, 0, len(path))
	for _, seg := range path {
		parts = append(parts, fmt.Sprintf("%v", seg))
	}
	return strings.Join(parts, " › ")
}

// buildTypeSubtree creates a type node and any required children (e.g., items child for array).
// Returns the root node and all nodes in the subtree.
func buildTypeSubtree(typeName string) (*projection.Node, []*projection.Node) {
	typeTemplate := schema.TypeTemplates[typeName]
	if typeTemplate == nil {
		typeTemplate = typeName
	}
	root := projection.NewNodeDetached(kindFromTypeName(typeName), typeTemplate, []any{})
	nodes := []*projection.Node{root}

	// Array and map need an items/values child node
	switch typeName {
	case "array":
		child := projection.NewNodeDetached(schema.KindPrimitive, "string", []any{})
		child.ParentID = root.ID
		root.Children = append(root.Children, child.ID)
		nodes = append(nodes, child)
	case "map":
		child := projection.NewNodeDetached(schema.KindPrimitive, "string", []any{})
		child.ParentID = root.ID
		root.Children = append(root.Children, child.ID)
		nodes = append(nodes, child)
	}
	return root, nodes
}

// kindFromTypeName maps an Avro type name to the corresponding NodeKind.
func kindFromTypeName(typeName string) schema.NodeKind {
	switch typeName {
	case "record":
		return schema.KindRecord
	case "enum":
		return schema.KindEnum
	case "fixed":
		return schema.KindFixed
	case "array":
		return schema.KindArray
	case "map":
		return schema.KindMap
	case "union":
		return schema.KindUnion
	default:
		if schema.IsPrimitive(typeName) {
			return schema.KindPrimitive
		}
		return schema.KindNamed
	}
}
