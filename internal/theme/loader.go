package theme

import (
	"encoding/json"
	"fmt"
	"image/color"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
)

// ThemeFile represents the JSON structure of a theme file.
type ThemeFile struct {
	Name string `json:"name"`

	// Base colors (hex strings)
	Fg    string `json:"fg"`
	Bg    string `json:"bg"`
	Dim   string `json:"dim"`
	Muted string `json:"muted"`

	// Accent colors
	Primary   string `json:"primary"`
	Secondary string `json:"secondary"`
	Success   string `json:"success"`
	Warning   string `json:"warning"`
	Error     string `json:"error"`

	// Kind badge colors (background)
	BadgeRecord string `json:"badge_record"`
	BadgeField  string `json:"badge_field"`
	BadgeEnum   string `json:"badge_enum"`
	BadgeArray  string `json:"badge_array"`
	BadgeMap    string `json:"badge_map"`
	BadgeUnion  string `json:"badge_union"`
	BadgeFixed  string `json:"badge_fixed"`
	BadgeNamed  string `json:"badge_named"`

	// Status bar 
	ModeNormal  string `json:"mode_normal"`
	ModeEdit    string `json:"mode_edit"`
	ModeSearch  string `json:"mode_search"`
	ModeCommand string `json:"mode_command"`
	StatusBg    string `json:"status_bg"`

	// Symbols (all optional — omitted values use defaults)
	Symbols *SymbolsFile `json:"symbols,omitempty"`
}

// SymbolsFile represents the optional symbols section in a theme JSON.
type SymbolsFile struct {
	Expanded     string `json:"expanded,omitempty"`
	Collapsed    string `json:"collapsed,omitempty"`
	Leaf         string `json:"leaf,omitempty"`
	NamedRef     string `json:"named_ref,omitempty"`
	Cursor       string `json:"cursor,omitempty"`
	DetailExpand string `json:"detail_expand,omitempty"`
	Error        string `json:"error,omitempty"`
	Warning      string `json:"warning,omitempty"`
	Success      string `json:"success,omitempty"`
	ScrollUp     string `json:"scroll_up,omitempty"`
	ScrollDown   string `json:"scroll_down,omitempty"`
}

// LoadFile reads a JSON theme file and builds a Theme.
func LoadFile(path string) (*Theme, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read theme: %w", err)
	}
	var tf ThemeFile
	if err := json.Unmarshal(data, &tf); err != nil {
		return nil, fmt.Errorf("parse theme: %w", err)
	}
	return tf.Build(), nil
}

// Build converts a ThemeFile into a fully resolved Theme.
func (tf *ThemeFile) Build() *Theme {
	fg := hex(tf.Fg, "#c0caf5")
	bg := hex(tf.Bg, "#1a1b26")
	dim := hex(tf.Dim, "#888ea8")
	muted := hex(tf.Muted, "#414868")

	primary := hex(tf.Primary, "#7aa2f7")
	secondary := hex(tf.Secondary, "#bb9af7")
	success := hex(tf.Success, "#9ece6a")
	warning := hex(tf.Warning, "#e0af68")
	errColor := hex(tf.Error, "#f7768e")

	badgeRecord := hex(tf.BadgeRecord, or(tf.Primary, "#7aa2f7"))
	badgeField := hex(tf.BadgeField, or(tf.Success, "#9ece6a"))
	badgeEnum := hex(tf.BadgeEnum, or(tf.Secondary, "#bb9af7"))
	badgeArray := hex(tf.BadgeArray, or(tf.Warning, "#e0af68"))
	badgeMap := hex(tf.BadgeMap, "#73daca")
	badgeUnion := hex(tf.BadgeUnion, "#ff9e64")
	badgeFixed := hex(tf.BadgeFixed, "#2ac3de")
	badgeNamed := hex(tf.BadgeNamed, or(tf.Secondary, "#bb9af7"))

	modeNormal := hex(tf.ModeNormal, "#7aa2f7")
	modeEdit := hex(tf.ModeEdit, "#9ece6a")
	modeSearch := hex(tf.ModeSearch, "#e0af68")
	modeCommand := hex(tf.ModeCommand, "#bb9af7")
	statusBg := hex(tf.StatusBg, "#24283b")

	borderStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(muted)

	focusedBorderStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(primary)

	name := tf.Name
	if name == "" {
		name = "custom"
	}

	sym := buildSymbols(tf.Symbols)

	return &Theme{
		Name: name,
		Sym:  sym,

		Fg:    fg,
		Bg:    bg,
		Dim:   dim,
		Muted: muted,

		Primary:   primary,
		Secondary: secondary,
		Success:   success,
		Warning:   warning,
		Error:     errColor,

		TreeBorder:    borderStyle,
		DetailBorder:  borderStyle,
		FocusedBorder: focusedBorderStyle,

		TreeNode: lipgloss.NewStyle().Foreground(fg),
		TreeNodeActive: lipgloss.NewStyle().
			Foreground(bg).
			Background(primary).
			Bold(true),
		TreeIndent:   lipgloss.NewStyle().Foreground(dim),
		TreeExpander: lipgloss.NewStyle().Foreground(dim),

		KindRecord: lipgloss.NewStyle().Foreground(badgeRecord).Bold(true),
		KindField:  lipgloss.NewStyle().Foreground(badgeField),
		KindEnum:   lipgloss.NewStyle().Foreground(badgeEnum),
		KindArray:  lipgloss.NewStyle().Foreground(badgeArray),
		KindMap:    lipgloss.NewStyle().Foreground(badgeMap),
		KindUnion:  lipgloss.NewStyle().Foreground(badgeUnion),
		KindFixed:  lipgloss.NewStyle().Foreground(badgeFixed),
		KindPrim:   lipgloss.NewStyle().Foreground(dim),
		KindNamed:  lipgloss.NewStyle().Foreground(badgeNamed),

		DetailKey:   lipgloss.NewStyle().Foreground(primary).Bold(true),
		DetailValue: lipgloss.NewStyle().Foreground(fg),
		DetailTitle: lipgloss.NewStyle().Foreground(secondary).Bold(true).Underline(true),

		StatusBar: lipgloss.NewStyle().
			Foreground(fg).
			Background(statusBg),
		StatusMode: lipgloss.NewStyle().
			Foreground(bg).
			Bold(true).
			Padding(0, 1),
		StatusFile:  lipgloss.NewStyle().Foreground(fg).Padding(0, 1),
		StatusStats: lipgloss.NewStyle().Foreground(dim).Padding(0, 1),
		StatusHelp:  lipgloss.NewStyle().Foreground(muted).Padding(0, 1),

		StatusNormal:  lipgloss.NewStyle().Background(modeNormal).Foreground(bg).Bold(true).Padding(0, 1),
		StatusEdit:    lipgloss.NewStyle().Background(modeEdit).Foreground(bg).Bold(true).Padding(0, 1),
		StatusSearch:  lipgloss.NewStyle().Background(modeSearch).Foreground(bg).Bold(true).Padding(0, 1),
		StatusCommand: lipgloss.NewStyle().Background(modeCommand).Foreground(bg).Bold(true).Padding(0, 1),
	}
}

// DiscoverThemes finds all .json theme files in a directory.
func DiscoverThemes(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".json") {
			name := strings.TrimSuffix(e.Name(), ".json")
			names = append(names, name)
		}
	}
	return names, nil
}

// ThemesDir returns the default themes directory path.
// Checks: CWD/themes → next to executable → ~/.config/avredit/themes.
func ThemesDir() string {
	// Check current working directory
	if info, err := os.Stat("themes"); err == nil && info.IsDir() {
		return "themes"
	}
	// Check next to executable
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Join(filepath.Dir(exe), "themes")
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	// Fall back to config dir
	home, err := os.UserHomeDir()
	if err != nil {
		return "themes"
	}
	return filepath.Join(home, ".config", "avredit", "themes")
}

// Registry holds available themes by name.
type Registry struct {
	themes map[string]*Theme
	dir    string
}

// NewRegistry creates a registry pre-loaded with built-in themes.
// It loads themes from the primary dir and also the user config dir.
func NewRegistry(dir string) *Registry {
	r := &Registry{
		themes: map[string]*Theme{
			"dark": Dark(),
		},
		dir: dir,
	}
	r.loadFromDir()
	// Also load from user config themes if different from primary
	userDir := userThemesDir()
	if userDir != "" && userDir != dir {
		r.loadFromPath(userDir)
	}
	return r
}

// Get returns a theme by name, or nil if not found.
func (r *Registry) Get(name string) *Theme {
	return r.themes[name]
}

// Names returns all available theme names.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.themes))
	for n := range r.themes {
		names = append(names, n)
	}
	return names
}

func (r *Registry) loadFromDir() {
	r.loadFromPath(r.dir)
}

func (r *Registry) loadFromPath(dir string) {
	if dir == "" {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("theme directory read failed", "dir", dir, "error", err)
		}
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		t, err := LoadFile(path)
		if err != nil {
			slog.Warn("theme file load failed", "path", path, "error", err)
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		if t.Name != "" {
			name = t.Name
		}
		r.themes[name] = t
	}
}

// userThemesDir returns ~/.config/avredit/themes if it exists.
func userThemesDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, ".config", "avredit", "themes")
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		return dir
	}
	return ""
}

// buildSymbols merges theme symbol overrides with defaults.
func buildSymbols(sf *SymbolsFile) Symbols {
	s := DefaultSymbols
	if sf == nil {
		return s
	}
	if sf.Expanded != "" {
		s.Expanded = sf.Expanded
	}
	if sf.Collapsed != "" {
		s.Collapsed = sf.Collapsed
	}
	if sf.Leaf != "" {
		s.Leaf = sf.Leaf
	}
	if sf.NamedRef != "" {
		s.NamedRef = sf.NamedRef
	}
	if sf.Cursor != "" {
		s.Cursor = sf.Cursor
	}
	if sf.DetailExpand != "" {
		s.DetailExpand = sf.DetailExpand
	}
	if sf.Error != "" {
		s.Error = sf.Error
	}
	if sf.Warning != "" {
		s.Warning = sf.Warning
	}
	if sf.Success != "" {
		s.Success = sf.Success
	}
	if sf.ScrollUp != "" {
		s.ScrollUp = sf.ScrollUp
	}
	if sf.ScrollDown != "" {
		s.ScrollDown = sf.ScrollDown
	}
	return s
}

// hex converts a hex string to a color.Color via lipgloss with a fallback default.
func hex(s, fallback string) color.Color {
	if s == "" {
		return lipgloss.Color(fallback)
	}
	return lipgloss.Color(s)
}

// or returns a if non-empty, else b.
func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
