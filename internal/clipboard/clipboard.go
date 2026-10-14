// Package clipboard reads and writes the system clipboard, dependency-free:
//
//  1. OSC52 via the terminal itself (charm's tea.SetClipboard) for writes —
//     works locally and over SSH, no subprocess, but silently no-ops on
//     terminals that don't support the escape sequence. OSC52 has no
//     standard read-back, so it's write-only.
//  2. A best-effort native tool (pbcopy/pbpaste, xclip, wl-copy/wl-paste,
//     xsel, clip/Get-Clipboard) for both reads and writes — reliable
//     locally, but useless over a remote session with no local clipboard
//     tool.
//
// Callers are expected to invoke both write mechanisms together (see
// internal/model's yank commands), since either one landing is a win and
// neither has a real cost when it doesn't. Reads (pasting *into* avredit)
// go through ReadNative as a fallback for terminals where bracketed-paste
// doesn't reach the app directly.
package clipboard

import (
	"os/exec"
	"runtime"
	"strings"
)

// WriteNative writes text to the system clipboard using a platform-native
// tool. Returns an error if no suitable tool is found or it fails to run;
// callers should treat that as non-fatal (OSC52 is the other half of the
// strategy).
func WriteNative(text string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "windows":
		cmd = exec.Command("cmd", "/c", "clip")
	case "linux":
		if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.Command("wl-copy")
		} else if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else {
			return errNoTool
		}
	default:
		return errNoTool
	}

	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// ReadNative reads text from the system clipboard using a platform-native
// tool. This is the explicit-paste counterpart to WriteNative — used as a
// fallback for terminals/sessions where bracketed-paste (the terminal
// pasting text to the app directly) isn't available or doesn't fire.
func ReadNative() (string, error) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbpaste")
	case "windows":
		cmd = exec.Command("powershell", "-NoProfile", "-Command", "Get-Clipboard")
	case "linux":
		if _, err := exec.LookPath("wl-paste"); err == nil {
			cmd = exec.Command("wl-paste", "-n")
		} else if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard", "-o")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--output")
		} else {
			return "", errNoTool
		}
	default:
		return "", errNoTool
	}

	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

var errNoTool = errNoClipboardTool{}

type errNoClipboardTool struct{}

func (errNoClipboardTool) Error() string {
	return "no native clipboard tool found (install xclip, wl-copy, or xsel on Linux)"
}
