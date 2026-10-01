// avredit — Avro schema editor TUI
package main

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/onereallylongname/avredit/internal/config"
	avio "github.com/onereallylongname/avredit/internal/io"
	"github.com/onereallylongname/avredit/internal/logging"
	"github.com/onereallylongname/avredit/internal/model"
	"github.com/onereallylongname/avredit/internal/registry"
)

// Version is set at build time via ldflags (e.g.
// -ldflags "-X main.Version=1.2.3"); defaults to this value otherwise.
var Version = "0.2.0"

func main() {
	// Parse flags manually (consistent with existing style)
	var logLevel string
	var target string
	var registryURL, registryToken, registryUser, registryPassword string
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		switch {
		case arg == "--version" || arg == "-v":
			printVersion()
			os.Exit(0)
		case arg == "--help" || arg == "-h":
			printUsage()
			os.Exit(0)
		case arg == "--log-level" && i+1 < len(os.Args):
			i++
			logLevel = os.Args[i]
		case strings.HasPrefix(arg, "--log-level="):
			logLevel = strings.TrimPrefix(arg, "--log-level=")
		case arg == "--registry-url" && i+1 < len(os.Args):
			i++
			registryURL = os.Args[i]
		case strings.HasPrefix(arg, "--registry-url="):
			registryURL = strings.TrimPrefix(arg, "--registry-url=")
		case arg == "--token" && i+1 < len(os.Args):
			i++
			registryToken = os.Args[i]
		case strings.HasPrefix(arg, "--token="):
			registryToken = strings.TrimPrefix(arg, "--token=")
		case arg == "--user" && i+1 < len(os.Args):
			i++
			registryUser = os.Args[i]
		case strings.HasPrefix(arg, "--user="):
			registryUser = strings.TrimPrefix(arg, "--user=")
		case arg == "--password" && i+1 < len(os.Args):
			i++
			registryPassword = os.Args[i]
		case strings.HasPrefix(arg, "--password="):
			registryPassword = strings.TrimPrefix(arg, "--password=")
		default:
			if target == "" {
				target = arg
			}
		}
	}

	cfg := config.Load()

	// Resolve log level: CLI flag > config > default (error)
	levelStr := cfg.LogLevel
	if logLevel != "" {
		levelStr = logLevel
	}
	if levelStr == "" {
		levelStr = "error"
	}
	level := logging.ParseLevel(levelStr)

	// Initialize logging
	cleanup, err := logging.Init(logging.LogDir(), level)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: logging init failed: %v\n", err)
	} else {
		defer cleanup()
	}

	// Resolve schema registry: CLI flag > config (URL only; credentials are
	// always session-only via flags and are never persisted to config).
	if registryURL == "" {
		registryURL = cfg.RegistryURL
	}
	regClient, regLabel := buildRegistryClient(registryURL, registryToken, registryUser, registryPassword)

	// Determine target: file, directory, or cwd
	if target == "" {
		target = "."
	}
	absTarget, _ := filepath.Abs(target)

	info, statErr := os.Stat(absTarget)
	if statErr != nil && target != "." {
		slog.Error("target not found", "path", absTarget, "error", statErr)
		fmt.Fprintf(os.Stderr, "Error: %v\n", statErr)
		os.Exit(1)
	}

	var app model.App
	if statErr == nil && info.IsDir() {
		app = model.NewAppExplorer(absTarget, cfg)
	} else {
		proj, _, loadErr := avio.LoadAvroFromFile(absTarget)
		if loadErr != nil {
			slog.Error("schema load failed", "path", absTarget, "error", loadErr)
			fmt.Fprintf(os.Stderr, "Error loading schema: %v\n", loadErr)
			os.Exit(1)
		}
		app = model.NewApp(proj, absTarget, cfg)
	}

	if regClient != nil {
		app = app.WithRegistryClient(regClient, regLabel)
	}

	p := tea.NewProgram(app)
	if _, err := p.Run(); err != nil {
		slog.Error("TUI runtime error", "error", err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// buildRegistryClient constructs a schema-registry client from session-only
// CLI credentials, if a registry URL was configured. Auth precedence: a
// bearer token takes priority over basic user/password; if neither is set
// the client makes unauthenticated requests (only viable for registries that
// don't require auth). Credentials never touch disk — they live only in
// this process's memory for the session.
func buildRegistryClient(registryURL, token, user, password string) (*registry.Client, string) {
	if registryURL == "" {
		return nil, ""
	}
	cfg := registry.Config{URL: registryURL}
	switch {
	case token != "":
		cfg.AuthType = registry.AuthBearer
		cfg.Token = token
	case user != "" || password != "":
		cfg.AuthType = registry.AuthBasic
		cfg.Username = user
		cfg.Password = password
	default:
		cfg.AuthType = registry.AuthNone
	}
	label := registryURL
	if u, err := url.Parse(registryURL); err == nil && u.Host != "" {
		label = u.Host
	}
	return registry.NewClient(cfg), label
}

// printVersion prints the version plus build-environment details (Go
// compiler version, OS/arch) that are always available from the Go runtime
// with no build-system or ldflags changes required — useful for bug reports
// and confirming which binary/platform combination is running.
func printVersion() {
	fmt.Printf("avredit v%s\n", Version)
	fmt.Printf("Go version: %s\n", runtime.Version())
	fmt.Printf("OS/Arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
}

func printUsage() {
	fmt.Println("avredit - Avro schema editor TUI")
	fmt.Println()
	fmt.Printf("Usage: avredit [flags] [<schema.avsc> | <directory>]\n\n")
	fmt.Println("  No arguments    Open in current directory with file explorer")
	fmt.Println("  <schema.avsc>   Open a specific schema file")
	fmt.Println("  <directory>     Open in directory with file explorer")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -h, --help              Show this help message")
	fmt.Println("  -v, --version           Show version information")
	fmt.Println("  --log-level <level>     Set log level (debug, info, warn, error)")
	fmt.Println("  --registry-url <url>    Schema Registry base URL (Confluent Cloud / Confluent-compatible)")
	fmt.Println("  --token <token>         Bearer token for registry auth (session-only, never saved)")
	fmt.Println("  --user <user>           Basic-auth username for registry auth (session-only)")
	fmt.Println("  --password <password>   Basic-auth password for registry auth (session-only)")
	fmt.Println()
	fmt.Print(model.KeybindingsText())
}
