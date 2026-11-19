# Changelog

## 0.3.0

### Added
- Mouse support: scroll wheel, left click (select, click again to expand/open), double click, and a right-click context menu.
- Clickable menus and pickers; clicking outside closes them.
- `Ctrl+P` opens the command line, with live command suggestions as you type.
- Fuzzy command autocomplete, plus argument completion for `:theme`, `:open`/`:e` and `:export`.
- `:help` and `:options` commands (the latter shows the right-click menu).
- Live theme preview in the theme picker; `Esc` restores the previous theme.
- Search: `|` for OR, space for AND between filters (e.g. `n:event t:string | d:deprecated`); `n:` also matches fields referencing a named type.

### Fixed
- Explorer and schema tree searches no longer share results; `n`/`N` follow the focused panel.
- `/` opens with an empty query (use `Up` for history).
- Help overlay: scrolling no longer overshoots the bottom, and the box size follows the window and wraps text.
