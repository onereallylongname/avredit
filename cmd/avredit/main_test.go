package main

import (
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

// TestVersionFlag_PrintsVersionAndBuildInfo verifies `--version`/`-v` print
// the version plus the Go compiler version and OS/arch, and exit
// successfully (0) without starting the TUI.
func TestVersionFlag_PrintsVersionAndBuildInfo(t *testing.T) {
	for _, flag := range []string{"--version", "-v"} {
		t.Run(flag, func(t *testing.T) {
			out, err := exec.Command("go", "run", ".", flag).CombinedOutput()
			if err != nil {
				t.Fatalf("running with %s failed: %v\noutput:\n%s", flag, err, out)
			}
			text := string(out)
			if !strings.Contains(text, "avredit v"+Version) {
				t.Errorf("expected output to contain %q, got:\n%s", "avredit v"+Version, text)
			}
			if !strings.Contains(text, runtime.Version()) {
				t.Errorf("expected output to contain the Go version %q, got:\n%s", runtime.Version(), text)
			}
			if !strings.Contains(text, runtime.GOOS+"/"+runtime.GOARCH) {
				t.Errorf("expected output to contain OS/Arch %q, got:\n%s", runtime.GOOS+"/"+runtime.GOARCH, text)
			}
		})
	}
}
