package model

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/onereallylongname/avredit/internal/command"
	"github.com/onereallylongname/avredit/internal/io"
	"github.com/onereallylongname/avredit/internal/registry"
)

// registryRequestTimeout bounds every registry network call issued from the
// UI so a slow/unreachable registry can't freeze the interface indefinitely
// (the http.Client itself also has its own Config.Timeout; this is an extra
// per-call ceiling for cancellation).
const registryRequestTimeout = 15 * time.Second

// --- tea.Msg types produced by registry tea.Cmd calls ---

type registrySubjectsMsg struct {
	subjects []string
	err      error
}

type registryVersionsMsg struct {
	subject  string
	versions []int
	err      error
}

type registrySchemaMsg struct {
	subject    string
	version    int
	schemaJSON string
	err        error
}

type registryCompatMsg struct {
	subject string
	result  *registry.CompatibilityResult
	diff    []string // structural (semantic) diff vs the registry's latest version; check_only scope
	err     error
}

// --- tea.Cmd builders (async; run off the UI goroutine) ---

func (a App) fetchRegistrySubjectsCmd() tea.Cmd {
	client := a.registryClient
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), registryRequestTimeout)
		defer cancel()
		subjects, err := client.ListSubjects(ctx)
		return registrySubjectsMsg{subjects: subjects, err: err}
	}
}

func (a App) fetchRegistryVersionsCmd(subject string) tea.Cmd {
	client := a.registryClient
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), registryRequestTimeout)
		defer cancel()
		versions, err := client.ListVersions(ctx, subject)
		return registryVersionsMsg{subject: subject, versions: versions, err: err}
	}
}

func (a App) fetchRegistrySchemaCmd(subject string, version int) tea.Cmd {
	client := a.registryClient
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), registryRequestTimeout)
		defer cancel()
		sv, err := client.GetSchema(ctx, subject, version)
		if err != nil {
			return registrySchemaMsg{subject: subject, version: version, err: err}
		}
		return registrySchemaMsg{subject: subject, version: sv.Version, schemaJSON: sv.Schema}
	}
}

// checkRegistryCompatCmd checks localSchemaJSON for compatibility against the
// subject's latest registered version, and (best-effort) computes a
// structural/semantic diff against that latest version for display alongside
// the result.
func (a App) checkRegistryCompatCmd(subject, localSchemaJSON string) tea.Cmd {
	client := a.registryClient
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), registryRequestTimeout)
		defer cancel()
		result, err := client.CheckCompatibility(ctx, subject, 0, localSchemaJSON)
		if err != nil {
			return registryCompatMsg{subject: subject, err: err}
		}
		var diff []string
		if remote, rerr := client.GetSchema(ctx, subject, 0); rerr == nil {
			diff = structuralDiff(remote.Schema, localSchemaJSON)
		}
		return registryCompatMsg{subject: subject, result: result, diff: diff}
	}
}

// --- App-level orchestration ---

// toggleRegistryMode switches the explorer pane between Files and Registry
// mode (ctrl+g). Registry mode requires a configured registry client
// (session-only, via --registry-url/--token/--user/--password flags).
func (a App) toggleRegistryMode() (tea.Model, tea.Cmd) {
	if a.registryClient == nil {
		a.flashError("No schema registry configured — start avredit with --registry-url (and --token or --user/--password)")
		return a, nil
	}
	if a.explorer.Mode() == ExplorerRegistry {
		a.explorer.SetMode(ExplorerFiles)
		a.updateLayout()
		return a, nil
	}
	a.explorer.SetMode(ExplorerRegistry)
	if !a.explorer.Visible() {
		a.explorer.Toggle()
	}
	a.focus = PanelExplorer
	a.updateLayout()
	if !a.explorer.HasRegistrySubjects() {
		a.explorer.SetRegistryLoading(true)
		a.flashInfo("Registry: loading subjects...")
		return a, a.fetchRegistrySubjectsCmd()
	}
	return a, nil
}

// beginRegistryFetch is invoked when the user selects a specific version row
// in the Registry explorer (enter). Like openFile, it guards against
// discarding unsaved changes.
func (a App) beginRegistryFetch(subject string, version int) (tea.Model, tea.Cmd) {
	if a.registryClient == nil {
		return a, nil
	}
	if a.dirty {
		a.pendingRegistrySubject = subject
		a.pendingRegistryVersion = version
		a.confirm = NewConfirm("Unsaved changes. Load anyway?", a.theme)
		a.overlay = OverlayConfirm
		a.overlayAction = ActionLoadRegistry
		return a, nil
	}
	a.flashInfo(fmt.Sprintf("Registry: fetching %s v%d...", subject, version))
	return a, a.fetchRegistrySchemaCmd(subject, version)
}

// openRegistrySchema loads a fetched registry schema into the editor buffer.
// Per the "no push" design decision, this never writes to disk on its own:
// filePath is cleared so a subsequent :w <path> is required to save,
// identical to how a brand-new unsaved buffer behaves.
func (a App) openRegistrySchema(subject string, version int, schemaJSON string) (tea.Model, tea.Cmd) {
	proj, _, err := io.LoadAvroFromBytes([]byte(schemaJSON))
	if err != nil {
		a.flashError("Registry: fetched schema is invalid: "+err.Error(), "subject", subject, "version", version, "error", err)
		return a, nil
	}
	a.proj = proj
	a.filePath = ""
	a.dirty = false
	a.history = command.NewHistory(500)
	a.tree = NewTreeModel(proj, a.theme)
	a.details = NewDetailsModel(proj, a.theme)
	a.statusbar.SetFile(fmt.Sprintf("registry:%s@v%d (unsaved)", subject, version))
	a.focus = PanelTree
	a.updateLayout()
	a.runValidation()
	a.flashInfo(fmt.Sprintf("Loaded %s v%d from registry — use :w <path> to save", subject, version))
	return a, nil
}

// executeRegistryCommand handles ":registry <subcommand> [args...]".
func (a App) executeRegistryCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		a.flashError("Usage: :registry check [subject] | :registry report")
		return a, nil
	}
	switch args[0] {
	case "check":
		if a.registryClient == nil {
			a.flashError("No schema registry configured — start avredit with --registry-url")
			return a, nil
		}
		subject := ""
		if len(args) >= 2 {
			subject = args[1]
		} else {
			s, err := a.inferRegistrySubject()
			if err != nil {
				a.flashError("Registry check: " + err.Error())
				return a, nil
			}
			subject = s
		}
		data, err := io.SerializeAvro(a.proj, false)
		if err != nil {
			a.flashError("Registry check: "+err.Error(), "error", err)
			return a, nil
		}
		a.flashInfo("Registry: checking compatibility for " + subject + "...")
		return a, a.checkRegistryCompatCmd(subject, string(data))
	case "report":
		if a.lastRegistryCheck == nil {
			a.flashError("No registry check has been run yet — use :registry check [subject]")
			return a, nil
		}
		a.overlay = OverlayRegistryReport
		a.registryReportScrl = 0
	default:
		a.flashError("Unknown registry subcommand: " + args[0])
	}
	return a, nil
}

// inferRegistrySubject derives a Confluent TopicNameStrategy-style subject
// name ("<namespace>.<name>-value") from the currently loaded schema's root
// record. Used when ":registry check" is run without an explicit subject.
func (a App) inferRegistrySubject() (string, error) {
	if a.proj == nil || a.proj.RootID == "" {
		return "", fmt.Errorf("no schema loaded")
	}
	root := a.proj.Get(a.proj.RootID)
	if root == nil {
		return "", fmt.Errorf("no schema loaded")
	}
	name := root.Name()
	if name == "" {
		return "", fmt.Errorf(`root schema has no "name" — cannot infer subject; use :registry check <subject>`)
	}
	full := name
	if ns := root.Namespace(); ns != "" {
		full = ns + "." + name
	}
	return full + "-value", nil
}

// RegistryCheckReport captures one ":registry check" result. Only the most
// recent report is ever kept (a new check simply replaces a.lastRegistryCheck)
// — there's no history/log to prune. It backs the OverlayRegistryReport
// overlay and can be persisted to disk on demand via saveRegistryReport.
type RegistryCheckReport struct {
	Subject   string
	Result    *registry.CompatibilityResult
	Diff      []string
	CheckedAt time.Time

	// entries/unmatched pair diff lines with verbose compatibility-check
	// reasons. Computed once here rather than on every render: the
	// overlay re-renders on every keypress while open (scrolling, save,
	// etc.), and recomputing the pairing from scratch each time added up
	// to noticeable input lag on large schemas.
	entries   []diffEntry
	unmatched []string
}

// newRegistryCheckReport builds a report, eagerly computing its diff/reason
// pairing once up front (see RegistryCheckReport.entries/unmatched).
func newRegistryCheckReport(subject string, result *registry.CompatibilityResult, diff []string) *RegistryCheckReport {
	entries, unmatched := matchReasonsToDiff(diff, filterErrorMessages(result.Messages))
	return &RegistryCheckReport{
		Subject:   subject,
		Result:    result,
		Diff:      diff,
		CheckedAt: time.Now(),
		entries:   entries,
		unmatched: unmatched,
	}
}

// verboseContextMessageRe matches Confluent's non-error, context-only
// verbose messages: request/schema context it echoes back alongside actual
// per-field errors (e.g. "{oldSchema: ...}", "{oldSchemaVersion: 2}",
// "{validateFields: 'true', compatibility: 'FORWARD_TRANSITIVE'}"). These
// carry no explanation of what's wrong — real per-field failures are the
// "{errorType:..., description:..., ...}" messages — so they're filtered out
// of the report entirely rather than shown as (or paired with) a reason.
var verboseContextMessageRe = regexp.MustCompile(`^\{\s*(oldSchema|oldSchemaVersion|newSchemaVersion|validateFields)\b`)

// filterErrorMessages drops verbose compatibility-check messages that are
// just echoed request/schema context rather than an explanation of an
// actual incompatibility.
func filterErrorMessages(messages []string) []string {
	out := make([]string, 0, len(messages))
	for _, m := range messages {
		if verboseContextMessageRe.MatchString(strings.TrimSpace(m)) {
			continue
		}
		out = append(out, m)
	}
	return out
}

// diffFieldNameRe extracts the quoted field name from a structuralDiff line
// (e.g. `- removed field "age"` -> "age"), for matching against verbose
// compatibility-check messages that mention the same field.
var diffFieldNameRe = regexp.MustCompile(`"([^"]+)"`)

// diffEntry pairs one structural diff line with whichever verbose
// compatibility-check message(s) explain *why* that change is a problem
// (matched by field name), so the report can show the cause right next to
// the change instead of as two disconnected lists.
type diffEntry struct {
	Line    string
	Reasons []string
}

// matchReasonsToDiff groups the registry's verbose failure messages with the
// structural diff line(s) they most likely explain, matched by the field
// name each diff line mentions appearing as a substring of the message.
// Messages that don't mention any diffed field name (e.g. subject-level or
// unrelated compatibility-mode failures) are returned separately as
// "unmatched" so nothing is silently dropped.
func matchReasonsToDiff(diff, messages []string) (entries []diffEntry, unmatched []string) {
	entries = make([]diffEntry, len(diff))
	used := make([]bool, len(messages))
	for i, d := range diff {
		entries[i] = diffEntry{Line: d}
		m := diffFieldNameRe.FindStringSubmatch(d)
		if len(m) < 2 {
			continue
		}
		name := m[1]
		for j, msg := range messages {
			if !used[j] && strings.Contains(msg, name) {
				entries[i].Reasons = append(entries[i].Reasons, msg)
				used[j] = true
			}
		}
	}
	for j, msg := range messages {
		if !used[j] {
			unmatched = append(unmatched, msg)
		}
	}
	return entries, unmatched
}

// maxReasonDisplayLen caps how much of a single verbose compatibility
// message is shown in the report. Confluent's registry can return messages
// that embed a large chunk of the old/new schema JSON (e.g. an
// "READER_FIELD_MISSING_DEFAULT_VALUE"-style message with an embedded
// oldSchema), which is unreadable as a single report line/paragraph and
// (before this cap existed) made every re-render of the overlay slow. The
// registry itself remains the source of truth for the untruncated message.
const maxReasonDisplayLen = 400

// truncateReason shortens a verbose reason string for display, appending an
// ellipsis and a rune count note when truncated so it's clear more text
// exists.
func truncateReason(s string) string {
	r := []rune(s)
	if len(r) <= maxReasonDisplayLen {
		return s
	}
	return string(r[:maxReasonDisplayLen]) + fmt.Sprintf("… (%d more chars)", len(r)-maxReasonDisplayLen)
}

// PlainText renders the report as a self-contained plain-text/markdown
// document, used when saving it to disk.
func (r *RegistryCheckReport) PlainText() string {
	var b strings.Builder
	verdict := "COMPATIBLE"
	if !r.Result.Compatible {
		verdict = "NOT COMPATIBLE"
	}
	fmt.Fprintf(&b, "# Registry compatibility check: %s\n\n", r.Subject)
	fmt.Fprintf(&b, "Checked at: %s\n", r.CheckedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "Result: %s\n\n", verdict)

	entries, unmatched := r.entries, r.unmatched
	if len(entries) == 0 {
		b.WriteString("(no structural differences from latest registered version)\n\n")
	} else {
		b.WriteString("## Structural diff vs latest registered version\n\n")
		for _, e := range entries {
			fmt.Fprintf(&b, "- %s\n", e.Line)
			for _, reason := range e.Reasons {
				fmt.Fprintf(&b, "    ↳ %s\n", truncateReason(reason))
			}
		}
		b.WriteString("\n")
	}
	if len(unmatched) > 0 {
		b.WriteString("## Other reasons\n\n")
		for _, m := range unmatched {
			fmt.Fprintf(&b, "- %s\n", truncateReason(m))
		}
	}
	return b.String()
}

// applyRegistryCompatResult records the compatibility result as the (single)
// last registry check report. A compatible result stays a quiet one-line
// flash message, matching every other routine confirmation in the app. An
// incompatible result additionally opens the OverlayRegistryReport overlay
// immediately, since that's the case the user actually needs to read
// carefully — flipping through ":notifications" for it was the original
// complaint this replaces.
func (a *App) applyRegistryCompatResult(subject string, msg registryCompatMsg) {
	a.lastRegistryCheck = newRegistryCheckReport(subject, msg.result, msg.diff)
	if msg.result.Compatible {
		a.flashInfo("Registry: " + subject + " is compatible " + a.theme.Sym.Success)
		return
	}
	a.flashWarn("Registry: " + subject + " is NOT compatible")
	a.overlay = OverlayRegistryReport
	a.registryReportScrl = 0
}

// saveRegistryReport writes the last registry-check report to disk. The
// filename is timestamped so repeated checks never clobber an earlier
// saved report.
func (a *App) saveRegistryReport() {
	if a.lastRegistryCheck == nil {
		a.flashError("No registry check report to save")
		return
	}
	r := a.lastRegistryCheck
	name := fmt.Sprintf("%s-registry-check-%s.md", sanitizeFilename(r.Subject), r.CheckedAt.Format("20060102-150405"))
	path := filepath.Join(a.explorer.RootDir(), name)
	if err := os.WriteFile(path, []byte(r.PlainText()), 0o644); err != nil {
		a.flashError("Failed to save registry report: "+err.Error(), "path", path, "error", err)
		return
	}
	a.flashInfo("Saved registry report to " + path)
}

// sanitizeFilename replaces characters that are invalid (or awkward) in
// filenames on common filesystems with "_".
func sanitizeFilename(s string) string {
	return strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_",
	).Replace(s)
}

// renderRegistryReportOverlay renders the last registry check result:
// subject, verdict, and the structural diff with each verbose failure
// reason shown directly under the change it explains (any reasons that
// couldn't be matched to a specific field are listed under "Other
// reasons") — scrollable, with a hint for closing and saving to disk.
func (a App) renderRegistryReportOverlay() string {
	r := a.lastRegistryCheck
	if r == nil {
		return "No registry check report available."
	}

	// Content width the overlay wraps to — bounded so the box (border +
	// padding, see renderOverlay) never exceeds the terminal, and capped
	// on very wide terminals so lines stay readable.
	contentWidth := a.width - 10
	if contentWidth > 100 {
		contentWidth = 100
	}
	if contentWidth < 20 {
		contentWidth = 20
	}

	diffStyle := lipgloss.NewStyle().Foreground(a.theme.Muted)
	reasonStyle := lipgloss.NewStyle().Foreground(a.theme.Warning)
	boldStyle := lipgloss.NewStyle().Bold(true)

	// styledBlock wraps plain text (with an optional indent prefix kept on
	// every wrapped line) to contentWidth *before* applying style, so word
	// wrap measures real character width rather than ANSI escape bytes.
	styledBlock := func(prefix, text string, style lipgloss.Style) []string {
		indent := strings.Repeat(" ", lipgloss.Width(prefix))
		avail := contentWidth - lipgloss.Width(prefix)
		if avail < 10 {
			avail = 10
		}
		wrapped := wrapPlainText(text, avail)
		out := make([]string, len(wrapped))
		for i, w := range wrapped {
			p := indent
			if i == 0 {
				p = prefix
			}
			out[i] = style.Render(p + w)
		}
		return out
	}

	var lines []string
	lines = append(lines, boldStyle.Foreground(a.theme.Primary).Render("Registry Check: "+r.Subject))
	if r.Result.Compatible {
		lines = append(lines, lipgloss.NewStyle().Foreground(a.theme.Success).Bold(true).Render(a.theme.Sym.Success+" Compatible"))
	} else {
		lines = append(lines, lipgloss.NewStyle().Foreground(a.theme.Error).Bold(true).Render(a.theme.Sym.Error+" NOT compatible"))
	}
	lines = append(lines, "")

	entries, unmatched := r.entries, r.unmatched
	lines = append(lines, boldStyle.Render("Structural diff vs latest registered version:"))
	if len(entries) == 0 {
		lines = append(lines, diffStyle.Render("  (no structural differences)"))
	} else {
		for _, e := range entries {
			lines = append(lines, styledBlock("  ", e.Line, diffStyle)...)
			for _, reason := range e.Reasons {
				lines = append(lines, styledBlock("    "+a.theme.Sym.Warning+" ", truncateReason(reason), reasonStyle)...)
			}
		}
	}

	if len(unmatched) > 0 {
		lines = append(lines, "", boldStyle.Render("Other reasons:"))
		for _, m := range unmatched {
			lines = append(lines, styledBlock("  "+a.theme.Sym.Warning+" ", truncateReason(m), reasonStyle)...)
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
	if a.registryReportScrl > maxScroll {
		a.registryReportScrl = maxScroll
	}
	end := a.registryReportScrl + maxLines
	if end > len(lines) {
		end = len(lines)
	}
	visible := lines[a.registryReportScrl:end]
	dimStyle := lipgloss.NewStyle().Foreground(a.theme.Muted).Italic(true)
	hint := "  esc close  j/k scroll  s save to disk"
	if a.registryReportScrl > 0 {
		hint += "  " + a.theme.Sym.ScrollUp
	}
	if a.registryReportScrl < maxScroll {
		hint += "  " + a.theme.Sym.ScrollDown
	}
	visible = append(visible, "", dimStyle.Render(hint))
	return strings.Join(visible, "\n")
}

// structuralDiff computes a field-level (semantic, not textual) diff between
// two Avro record schemas: which top-level fields were added or removed.
// This is intentionally shallow — it does not attempt to describe *how* a
// shared field's type changed. Comparing nested record/union types in full
// either loses detail (a short summary can't tell "record Foo" from
// "record Foo with a different nested field") or produces multi-kilobyte,
// unreadable dumps of the entire nested schema (docs, glossary tags,
// lineage metadata and all). The registry's own verbose compatibility
// messages (result.Messages) already explain *why* a change is a problem in
// human terms, and are paired with these add/remove lines (or listed under
// "Other reasons") by matchReasonsToDiff — that's the actual explanation
// the report leans on. Best-effort: unparsable or non-record schemas yield
// a nil diff (the compatibility result itself is still shown).
func structuralDiff(remoteJSON, localJSON string) []string {
	var remote, local map[string]any
	if json.Unmarshal([]byte(remoteJSON), &remote) != nil {
		return nil
	}
	if json.Unmarshal([]byte(localJSON), &local) != nil {
		return nil
	}

	remoteFields := schemaFieldMap(remote)
	localFields := schemaFieldMap(local)
	if remoteFields == nil && localFields == nil {
		return nil
	}

	seen := map[string]bool{}
	names := make([]string, 0, len(remoteFields)+len(localFields))
	for name := range remoteFields {
		names = append(names, name)
		seen[name] = true
	}
	for name := range localFields {
		if !seen[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	var diffs []string
	for _, name := range names {
		_, rOk := remoteFields[name]
		_, lOk := localFields[name]
		switch {
		case rOk && !lOk:
			diffs = append(diffs, fmt.Sprintf("- removed field %q", name))
		case !rOk && lOk:
			diffs = append(diffs, fmt.Sprintf("+ added field %q", name))
		}
	}
	return diffs
}

// schemaFieldMap extracts an Avro record schema's "fields" array into a
// name-keyed map. Returns nil if the schema isn't a record (no diff to show).
func schemaFieldMap(s map[string]any) map[string]map[string]any {
	fields, ok := s["fields"].([]any)
	if !ok {
		return nil
	}
	out := map[string]map[string]any{}
	for _, f := range fields {
		fm, ok := f.(map[string]any)
		if !ok {
			continue
		}
		if name, _ := fm["name"].(string); name != "" {
			out[name] = fm
		}
	}
	return out
}
