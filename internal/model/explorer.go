package model

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/onereallylongname/avredit/internal/theme"
)

// ExplorerMode selects what data source the explorer pane is browsing. The
// pane is modal: navigation, filtering, and rendering are shared between
// modes, only the data backing e.entries differs.
type ExplorerMode int

const (
	ExplorerFiles    ExplorerMode = iota // browsing the local filesystem
	ExplorerRegistry                     // browsing a Schema Registry's subjects/versions
)

// FileEntry represents a row in the explorer tree. In ExplorerFiles mode
// this is a filesystem file or directory; in ExplorerRegistry mode it's a
// registry subject (IsDir-like, expandable) or a specific version of a
// subject (leaf, selectable).
type FileEntry struct {
	Name     string
	Path     string // disk path (Files mode); unused in Registry mode
	IsDir    bool   // Files: is a directory. Registry: is a subject (expandable)
	Depth    int
	Expanded bool

	// Registry-mode metadata; zero values in Files mode.
	RegSubject string // set for both subject and version rows
	RegVersion int    // >0 for a version row; 0 for a subject row
	RegLoading bool   // true for a synthetic non-selectable "loading…" row
}

// ExplorerActionKind identifies what EnterSelected wants the caller to do.
type ExplorerActionKind int

const (
	ExplorerActionNone        ExplorerActionKind = iota
	ExplorerActionOpenFile                        // Files mode: open Path
	ExplorerActionFetchSchema                     // Registry mode: fetch RegSubject @ RegVersion
)

// ExplorerAction describes what the caller should do in response to a
// navigation key (enter/space/l), since fetching registry data requires
// async I/O the ExplorerModel itself can't perform.
type ExplorerAction struct {
	Kind    ExplorerActionKind
	Path    string
	Subject string
	Version int
}

// ExplorerModel manages the file/registry explorer panel.
type ExplorerModel struct {
	theme  *theme.Theme
	width  int
	height int

	rootDir string
	entries []FileEntry
	cursor  int
	offset  int
	visible bool

	// Search/filter state
	filterQuery  string
	matchIndices []int // indices into entries that match filter
	matchIdx     int   // current match position within matchIndices

	// Registry-mode state. Populated by the App from async fetch results;
	// ExplorerModel itself never performs network I/O.
	mode                ExplorerMode
	regLabel            string          // display label for the panel title, e.g. registry host
	regLoading          bool            // true while a subjects/versions fetch is in flight
	regSubjects         []string        // cached full subject list
	regVersions         map[string][]int // cached versions per subject, populated lazily
	regExpanded         map[string]bool  // which subjects are expanded
	pendingFetchSubject string           // set when a subject was expanded but has no cached versions yet
}

// NewExplorerModel creates a file explorer rooted at the given directory.
func NewExplorerModel(th *theme.Theme, rootDir string) ExplorerModel {
	// Always use absolute path for reliable file operations
	abs, err := filepath.Abs(rootDir)
	if err == nil {
		rootDir = abs
	}
	e := ExplorerModel{
		theme:   th,
		rootDir: rootDir,
		visible: false,
	}
	e.rebuild()
	return e
}

// Visible returns whether the explorer is shown.
func (e ExplorerModel) Visible() bool { return e.visible }

// Mode returns the current explorer mode (Files or Registry).
func (e ExplorerModel) Mode() ExplorerMode { return e.mode }

// Title returns the panel title reflecting the current mode, for display in
// the bordered panel header.
func (e ExplorerModel) Title() string {
	if e.mode == ExplorerRegistry {
		if e.regLabel != "" {
			return "Registry: " + e.regLabel
		}
		return "Registry"
	}
	return "Files"
}

// SetRegistryLabel sets the display label shown in the Registry mode title
// (typically the registry host).
func (e *ExplorerModel) SetRegistryLabel(label string) { e.regLabel = label }

// SetMode switches between Files and Registry mode. Switching to Registry
// mode does not itself perform any fetch; the caller (App) is responsible
// for triggering a subjects fetch (e.g. via SetRegistryLoading(true) followed
// by SetRegistrySubjects once the async call completes) the first time.
func (e *ExplorerModel) SetMode(m ExplorerMode) {
	if e.mode == m {
		return
	}
	e.mode = m
	e.cursor = 0
	e.offset = 0
	e.ClearFilter()
	e.rebuild()
}

// RegistryLoading reports whether a registry fetch is in flight.
func (e ExplorerModel) RegistryLoading() bool { return e.regLoading }

// SetRegistryLoading marks whether a registry fetch is in flight, so the
// view can show a loading indicator.
func (e *ExplorerModel) SetRegistryLoading(loading bool) {
	e.regLoading = loading
	if e.mode == ExplorerRegistry {
		e.rebuild()
	}
}

// HasRegistrySubjects reports whether the subject list has been fetched at
// least once this session.
func (e ExplorerModel) HasRegistrySubjects() bool { return e.regSubjects != nil }

// RegistrySubjects returns the cached subject list (nil if none fetched
// yet). Used e.g. by command-line tab completion for ":registry check".
func (e ExplorerModel) RegistrySubjects() []string { return e.regSubjects }

// SetRegistrySubjects caches the full subject list (fetched by the caller)
// and rebuilds the visible tree.
func (e *ExplorerModel) SetRegistrySubjects(subjects []string) {
	sorted := append([]string(nil), subjects...)
	sort.Strings(sorted)
	e.regSubjects = sorted
	e.regLoading = false
	if e.regExpanded == nil {
		e.regExpanded = make(map[string]bool)
	}
	if e.regVersions == nil {
		e.regVersions = make(map[string][]int)
	}
	e.cursor = 0
	e.offset = 0
	e.rebuild()
}

// SetRegistryVersions caches the version list for a subject (fetched by the
// caller after the subject was expanded) and rebuilds the visible tree.
func (e *ExplorerModel) SetRegistryVersions(subject string, versions []int) {
	if e.regVersions == nil {
		e.regVersions = make(map[string][]int)
	}
	sorted := append([]int(nil), versions...)
	sort.Ints(sorted)
	e.regVersions[subject] = sorted
	e.rebuild()
	// Restore cursor onto the subject row so the newly revealed versions
	// appear directly below the current selection.
	for i, en := range e.entries {
		if en.RegSubject == subject && en.RegVersion == 0 {
			e.cursor = i
			break
		}
	}
	e.adjustScroll()
}

// TakePendingFetch returns and clears the subject name (if any) that needs
// its version list fetched, set when the user expanded a subject that has
// no cached versions yet.
func (e *ExplorerModel) TakePendingFetch() (string, bool) {
	s := e.pendingFetchSubject
	e.pendingFetchSubject = ""
	if s == "" {
		return "", false
	}
	return s, true
}

// RootDir returns the explorer's current root directory.
func (e ExplorerModel) RootDir() string { return e.rootDir }

// Toggle shows/hides the explorer. When showing, refreshes the current
// listing (files or registry, depending on mode).
func (e *ExplorerModel) Toggle() {
	e.visible = !e.visible
	if e.visible {
		e.rebuild()
	}
}

// Refresh rebuilds the current listing. In Files mode this rescans disk; in
// Registry mode it just re-renders from cache (use the caller's registry
// refetch + SetRegistrySubjects to actually refresh from the network).
func (e *ExplorerModel) Refresh() { e.rebuild() }

// SetSize updates panel dimensions.
func (e *ExplorerModel) SetSize(w, h int) {
	e.width = w
	e.height = h
}

// SelectedEntry returns the currently selected file entry, or nil.
func (e *ExplorerModel) SelectedEntry() *FileEntry {
	if len(e.entries) == 0 || e.cursor < 0 || e.cursor >= len(e.entries) {
		return nil
	}
	return &e.entries[e.cursor]
}

// SelectedDisplayName returns the full, untruncated name of the entry under
// the cursor (a filename, directory name, subject, or "subject@vN"), for
// display outside the panel where names may otherwise be cut off to fit its
// width. Returns "" if nothing is selected.
func (e ExplorerModel) SelectedDisplayName() string {
	if len(e.entries) == 0 || e.cursor < 0 || e.cursor >= len(e.entries) {
		return ""
	}
	entry := e.entries[e.cursor]
	if entry.RegVersion != 0 {
		return fmt.Sprintf("%s@v%d", entry.RegSubject, entry.RegVersion)
	}
	return entry.Name
}

// HandleKey processes navigation keys for the explorer.
func (e ExplorerModel) HandleKey(msg tea.KeyPressMsg) ExplorerModel {
	switch msg.String() {
	case "j", "down":
		if e.cursor < len(e.entries)-1 {
			e.cursor++
			e.adjustScroll()
		}
	case "k", "up":
		if e.cursor > 0 {
			e.cursor--
			e.adjustScroll()
		}
	case "g", "home":
		e.cursor = 0
		e.offset = 0
	case "G", "end":
		e.cursor = len(e.entries) - 1
		e.adjustScroll()
	case "l", "right", "space":
		e.expandSelected()
	case "h", "left":
		e.collapseSelected()
	case "backspace", "-":
		if e.mode == ExplorerFiles {
			e.goUp()
		}
	case ".":
		if e.mode == ExplorerFiles {
			e.setRootToSelected()
		}
	}
	return e
}

// setRootToSelected sets the selected directory as the new explorer root.
func (e *ExplorerModel) setRootToSelected() {
	entry := e.SelectedEntry()
	if entry == nil {
		return
	}
	var newRoot string
	if entry.IsDir && entry.Name != ".." {
		newRoot = entry.Path
	} else if entry.IsDir && entry.Name == ".." {
		newRoot = entry.Path
	} else {
		// File selected — use its parent dir
		newRoot = filepath.Dir(entry.Path)
	}
	if newRoot == e.rootDir {
		return
	}
	e.rootDir = newRoot
	e.cursor = 0
	e.offset = 0
	e.rebuild()
}

// EnterSelected acts on the currently selected row and reports what the
// caller (App) should do, since fetching over the network requires async
// I/O the ExplorerModel can't perform itself.
func (e *ExplorerModel) EnterSelected() ExplorerAction {
	if e.mode == ExplorerRegistry {
		return e.enterSelectedRegistry()
	}
	return e.enterSelectedFiles()
}

func (e *ExplorerModel) enterSelectedFiles() ExplorerAction {
	entry := e.SelectedEntry()
	if entry == nil {
		return ExplorerAction{}
	}
	// ".." entry navigates to parent
	if entry.Name == ".." {
		e.goUp()
		return ExplorerAction{}
	}
	if entry.IsDir {
		e.toggleExpand(e.cursor)
		return ExplorerAction{}
	}
	// Only return .avsc files
	if strings.HasSuffix(strings.ToLower(entry.Name), ".avsc") {
		return ExplorerAction{Kind: ExplorerActionOpenFile, Path: entry.Path}
	}
	return ExplorerAction{}
}

func (e *ExplorerModel) enterSelectedRegistry() ExplorerAction {
	entry := e.SelectedEntry()
	if entry == nil || entry.RegLoading {
		return ExplorerAction{}
	}
	if entry.RegVersion == 0 {
		// Subject row: expand/collapse, requesting a versions fetch if needed.
		e.toggleExpand(e.cursor)
		return ExplorerAction{}
	}
	// Version row: request the caller fetch the actual schema content.
	return ExplorerAction{Kind: ExplorerActionFetchSchema, Subject: entry.RegSubject, Version: entry.RegVersion}
}

// goUp navigates to the parent directory.
func (e *ExplorerModel) goUp() {
	parent := filepath.Dir(e.rootDir)
	if parent == e.rootDir {
		return // already at root
	}
	e.rootDir = parent
	e.cursor = 0
	e.offset = 0
	e.rebuild()
}

func (e *ExplorerModel) expandSelected() {
	entry := e.SelectedEntry()
	if entry == nil || !entry.IsDir {
		return
	}
	if !entry.Expanded {
		e.toggleExpand(e.cursor)
	}
}

func (e *ExplorerModel) collapseSelected() {
	entry := e.SelectedEntry()
	if entry == nil {
		return
	}
	if entry.IsDir && entry.Expanded {
		e.toggleExpand(e.cursor)
		return
	}
	// If on a file or collapsed dir, jump to parent directory
	if entry.Depth > 0 {
		for i := e.cursor - 1; i >= 0; i-- {
			if e.entries[i].IsDir && e.entries[i].Depth < entry.Depth {
				e.cursor = i
				e.adjustScroll()
				return
			}
		}
	}
}

// entryKey returns a stable identity for an entry, used to relocate the
// cursor after a rebuild reshuffles row indices. Files are identified by
// disk path; registry rows by subject(+version).
func entryKey(en FileEntry) string {
	if en.RegSubject != "" || en.RegVersion != 0 {
		return "reg:" + en.RegSubject + "@" + strconv.Itoa(en.RegVersion)
	}
	return en.Path
}

func (e *ExplorerModel) toggleExpand(idx int) {
	if idx < 0 || idx >= len(e.entries) {
		return
	}
	entry := &e.entries[idx]
	if !entry.IsDir {
		return
	}
	entry.Expanded = !entry.Expanded
	key := entryKey(*entry)

	if e.mode == ExplorerRegistry && entry.Expanded && entry.RegVersion == 0 {
		if e.regExpanded == nil {
			e.regExpanded = make(map[string]bool)
		}
		e.regExpanded[entry.RegSubject] = true
		if _, cached := e.regVersions[entry.RegSubject]; !cached {
			e.pendingFetchSubject = entry.RegSubject
		}
	} else if e.mode == ExplorerRegistry && !entry.Expanded && entry.RegVersion == 0 {
		if e.regExpanded != nil {
			e.regExpanded[entry.RegSubject] = false
		}
	}

	e.rebuild()
	// Restore cursor position (entry might have shifted)
	for i, en := range e.entries {
		if entryKey(en) == key {
			e.cursor = i
			break
		}
	}
	e.adjustScroll()
}

func (e *ExplorerModel) adjustScroll() {
	viewH := e.height - 2 // account for borders
	if viewH < 1 {
		viewH = 1
	}
	if e.cursor < e.offset {
		e.offset = e.cursor
	}
	if e.cursor >= e.offset+viewH {
		e.offset = e.cursor - viewH + 1
	}
}

// rebuild regenerates the flat entry list from the active mode's data
// source (disk or cached registry data).
func (e *ExplorerModel) rebuild() {
	if e.mode == ExplorerRegistry {
		e.rebuildRegistry()
		return
	}
	e.rebuildFiles()
}

// rebuildRegistry regenerates entries from cached subjects/versions.
func (e *ExplorerModel) rebuildRegistry() {
	e.entries = nil
	if e.regLoading && len(e.regSubjects) == 0 {
		e.entries = append(e.entries, FileEntry{Name: "Loading subjects…", RegLoading: true})
		e.reapplyFilter()
		return
	}
	for _, subj := range e.regSubjects {
		expanded := e.regExpanded[subj]
		e.entries = append(e.entries, FileEntry{
			Name:       subj,
			IsDir:      true,
			Depth:      0,
			Expanded:   expanded,
			RegSubject: subj,
		})
		if !expanded {
			continue
		}
		versions, cached := e.regVersions[subj]
		if !cached {
			e.entries = append(e.entries, FileEntry{Name: "Loading versions…", Depth: 1, RegLoading: true})
			continue
		}
		for _, v := range versions {
			label := fmt.Sprintf("v%d", v)
			if v == versions[len(versions)-1] {
				label += " (latest)"
			}
			e.entries = append(e.entries, FileEntry{
				Name:       label,
				Depth:      1,
				RegSubject: subj,
				RegVersion: v,
			})
		}
	}
	e.reapplyFilter()
}

// rebuildFiles regenerates the flat entry list from disk.
func (e *ExplorerModel) rebuildFiles() {
	// Keep track of which directories were expanded
	expanded := make(map[string]bool)
	for _, en := range e.entries {
		if en.IsDir && en.Expanded {
			expanded[en.Path] = true
		}
	}

	e.entries = nil

	// Add ".." entry if not at filesystem root
	parent := filepath.Dir(e.rootDir)
	if parent != e.rootDir {
		e.entries = append(e.entries, FileEntry{
			Name:  "..",
			Path:  parent,
			IsDir: true,
			Depth: 0,
		})
	}

	e.buildDir(e.rootDir, 0, expanded)
	e.reapplyFilter()
}

func (e *ExplorerModel) buildDir(dir string, depth int, expanded map[string]bool) {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	// Sort: directories first, then files, alphabetical within each group
	var dirs, files []os.DirEntry
	for _, de := range dirEntries {
		name := de.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if de.IsDir() {
			dirs = append(dirs, de)
		} else {
			ext := strings.ToLower(filepath.Ext(name))
			if ext == ".avsc" || ext == ".json" {
				files = append(files, de)
			}
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name() < dirs[j].Name() })
	sort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })

	for _, d := range dirs {
		path := filepath.Join(dir, d.Name())
		if !dirHasRelevantFiles(path) {
			continue
		}
		isExpanded := expanded[path]
		e.entries = append(e.entries, FileEntry{
			Name:     d.Name(),
			Path:     path,
			IsDir:    true,
			Depth:    depth,
			Expanded: isExpanded,
		})
		if isExpanded {
			e.buildDir(path, depth+1, expanded)
		}
	}
	for _, f := range files {
		e.entries = append(e.entries, FileEntry{
			Name:  f.Name(),
			Path:  filepath.Join(dir, f.Name()),
			IsDir: false,
			Depth: depth,
		})
	}
}

// dirHasRelevantFiles checks recursively if a directory contains any .avsc or .json files.
func dirHasRelevantFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, de := range entries {
		name := de.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if de.IsDir() {
			if dirHasRelevantFiles(filepath.Join(dir, name)) {
				return true
			}
		} else {
			ext := strings.ToLower(filepath.Ext(name))
			if ext == ".avsc" || ext == ".json" {
				return true
			}
		}
	}
	return false
}

// Filter applies a search query, matching the active mode's semantics:
// in Files mode it walks the filesystem (expanding ancestor dirs of any
// match); in Registry mode it matches against the already-fetched subject
// list only (no network calls are triggered from here).
func (e *ExplorerModel) Filter(query string) {
	if e.mode == ExplorerRegistry {
		e.filterRegistry(query)
		return
	}
	e.filterFiles(query)
}

// filterRegistry matches the query against currently visible rows (cached
// subjects, plus any already-expanded versions). Expanding further subjects
// to search their versions would require a network fetch, which
// ExplorerModel never performs itself.
func (e *ExplorerModel) filterRegistry(query string) {
	e.filterQuery = strings.ToLower(query)
	e.matchIndices = nil
	e.matchIdx = 0
	if e.filterQuery == "" {
		return
	}
	for i, entry := range e.entries {
		if strings.Contains(strings.ToLower(entry.Name), e.filterQuery) {
			e.matchIndices = append(e.matchIndices, i)
		}
	}
	if len(e.matchIndices) > 0 {
		e.cursor = e.matchIndices[0]
		e.adjustScroll()
	}
}

// filterFiles applies a search query recursively and expands directories to show matches.
func (e *ExplorerModel) filterFiles(query string) {
	e.filterQuery = strings.ToLower(query)
	e.matchIndices = nil
	e.matchIdx = 0
	if e.filterQuery == "" {
		return
	}

	// Walk the filesystem to find all matching files/dirs.
	// Collect paths to expand (ancestor dirs of matches).
	dirsToExpand := make(map[string]bool)
	_ = filepath.WalkDir(e.rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			ext := strings.ToLower(filepath.Ext(name))
			if ext != ".avsc" && ext != ".json" {
				return nil
			}
		}
		if strings.Contains(strings.ToLower(name), e.filterQuery) {
			// Mark all ancestor directories for expansion
			p := filepath.Dir(path)
			for {
				if p == e.rootDir || p == "." || p == string(filepath.Separator) {
					break
				}
				// Safety: stop if we've gone above rootDir
				rel, err := filepath.Rel(e.rootDir, p)
				if err != nil || strings.HasPrefix(rel, "..") {
					break
				}
				dirsToExpand[p] = true
				next := filepath.Dir(p)
				if next == p {
					break
				}
				p = next
			}
		}
		return nil
	})

	if len(dirsToExpand) == 0 && e.filterQuery != "" {
		// No matches found — still rebuild match indices from current entries
		for i, entry := range e.entries {
			if strings.Contains(strings.ToLower(entry.Name), e.filterQuery) {
				e.matchIndices = append(e.matchIndices, i)
			}
		}
		return
	}

	// Merge with currently expanded directories
	expanded := make(map[string]bool)
	for _, en := range e.entries {
		if en.IsDir && en.Expanded {
			expanded[en.Path] = true
		}
	}
	for p := range dirsToExpand {
		expanded[p] = true
	}

	// Rebuild entries with expanded dirs
	e.entries = nil
	parent := filepath.Dir(e.rootDir)
	if parent != e.rootDir {
		e.entries = append(e.entries, FileEntry{
			Name: "..", Path: parent, IsDir: true, Depth: 0,
		})
	}
	e.buildDir(e.rootDir, 0, expanded)

	// Build match indices for entries whose name matches the query
	for i, entry := range e.entries {
		if strings.Contains(strings.ToLower(entry.Name), e.filterQuery) {
			e.matchIndices = append(e.matchIndices, i)
		}
	}
	if len(e.matchIndices) > 0 {
		e.cursor = e.matchIndices[0]
		e.adjustScroll()
	}
}

// reapplyFilter re-runs the current filter after a structural change.
func (e *ExplorerModel) reapplyFilter() {
	if e.filterQuery == "" {
		return
	}
	e.matchIndices = nil
	e.matchIdx = 0
	for i, entry := range e.entries {
		if strings.Contains(strings.ToLower(entry.Name), e.filterQuery) {
			e.matchIndices = append(e.matchIndices, i)
		}
	}
}

// ClearFilter removes the current filter.
func (e *ExplorerModel) ClearFilter() {
	e.filterQuery = ""
	e.matchIndices = nil
	e.matchIdx = 0
}

// FilteredCount returns the number of matches.
func (e *ExplorerModel) FilteredCount() int {
	return len(e.matchIndices)
}

// FilterIdx returns the current match index (0-based).
func (e *ExplorerModel) FilterIdx() int {
	return e.matchIdx
}

// NextMatch moves to the next filter match.
func (e *ExplorerModel) NextMatch() {
	if len(e.matchIndices) == 0 {
		return
	}
	e.matchIdx = (e.matchIdx + 1) % len(e.matchIndices)
	e.cursor = e.matchIndices[e.matchIdx]
	e.adjustScroll()
}

// PrevMatch moves to the previous filter match.
func (e *ExplorerModel) PrevMatch() {
	if len(e.matchIndices) == 0 {
		return
	}
	e.matchIdx--
	if e.matchIdx < 0 {
		e.matchIdx = len(e.matchIndices) - 1
	}
	e.cursor = e.matchIndices[e.matchIdx]
	e.adjustScroll()
}

// View renders the file explorer panel.
func (e ExplorerModel) View() string {
	if len(e.entries) == 0 {
		return "  (empty)"
	}

	viewH := e.height - 2
	if viewH < 1 {
		viewH = 1
	}

	var b strings.Builder
	end := e.offset + viewH
	if end > len(e.entries) {
		end = len(e.entries)
	}

	for i := e.offset; i < end; i++ {
		if i > e.offset {
			b.WriteByte('\n')
		}
		entry := e.entries[i]
		indent := strings.Repeat("  ", entry.Depth)

		var icon string
		switch {
		case entry.RegLoading:
			icon = "… "
		case entry.RegSubject != "" && entry.RegVersion == 0:
			// Registry subject row (expandable, like a directory)
			if entry.Expanded {
				icon = " "
			} else {
				icon = " "
			}
		case entry.RegVersion != 0:
			// Registry version row (leaf, selectable)
			icon = "󰈙 "
		case entry.IsDir:
			if entry.Expanded {
				icon = " "
			} else {
				icon = " "
			}
		default:
			ext := strings.ToLower(filepath.Ext(entry.Name))
			if ext == ".avsc" {
				icon = "󰈙 "
			} else {
				icon = " "
			}
		}

		line := indent + icon + entry.Name

		// Truncate to width
		maxW := e.width - 2
		if maxW < 4 {
			maxW = 4
		}
		if len(line) > maxW {
			line = line[:maxW-1] + "…"
		}
		// Pad to width
		if len(line) < maxW {
			line += strings.Repeat(" ", maxW-len(line))
		}

		if i == e.cursor {
			b.WriteString(e.theme.TreeNodeActive.Render(line))
		} else if e.isMatch(i) {
			b.WriteString(lipgloss.NewStyle().Foreground(e.theme.Warning).Render(line))
		} else if entry.IsDir {
			b.WriteString(lipgloss.NewStyle().Foreground(e.theme.Primary).Render(line))
		} else {
			b.WriteString(line)
		}
	}

	return b.String()
}

// isMatch returns true if the entry at index i is a filter match.
func (e ExplorerModel) isMatch(i int) bool {
	for _, idx := range e.matchIndices {
		if idx == i {
			return true
		}
	}
	return false
}
