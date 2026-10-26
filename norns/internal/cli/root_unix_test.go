//go:build !windows

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Ghr07h/norns/norns/internal/version"
)

func TestCLIExternalExtensionPassesThroughProcessBoundary(t *testing.T) {
	workspace := t.TempDir()
	pluginDir := t.TempDir()
	t.Chdir(workspace)
	plugin := filepath.Join(pluginDir, "norns-probe")
	script := `#!/bin/sh
printf 'plugin-out:%s|%s|%s|%s|%s\n' "$1" "$NORNS_PROTOCOL_VERSION" "$NORNS_SCHEMA_VERSION" "$NORNS_WORKSPACE" "$NORNS_EXECUTABLE"
printf 'plugin-err\n' >&2
exit 23
`
	if err := os.WriteFile(plugin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NORNS_PLUGIN_PATH", pluginDir)
	t.Setenv("PATH", t.TempDir())

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"probe", "argument with spaces"}, strings.NewReader("input"), &stdout, &stderr)
	if code != 23 {
		t.Fatalf("extension exit code was wrapped: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if stderr.String() != "plugin-err\n" {
		t.Fatalf("extension stderr was wrapped or changed: %q", stderr.String())
	}
	parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(stdout.String(), "plugin-out:"), "\n"), "|")
	if len(parts) != 5 || parts[0] != "argument with spaces" || parts[1] != strconv.Itoa(version.ProtocolVersion) || parts[2] != strconv.Itoa(version.SchemaVersion) || parts[3] != workspace || parts[4] == "" {
		t.Fatalf("extension arguments or environment are wrong: %q", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(workspace, ".norns.lock")); !os.IsNotExist(err) {
		t.Fatalf("extension dispatch accessed the Workspace lifecycle lock: %v", err)
	}
}

func TestCLICoreCommandCannotBeOverriddenByExtension(t *testing.T) {
	workspace := t.TempDir()
	pluginDir := t.TempDir()
	t.Chdir(workspace)
	plugin := filepath.Join(pluginDir, "norns-version")
	if err := os.WriteFile(plugin, []byte("#!/bin/sh\nprintf 'extension override'\nexit 41\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NORNS_PLUGIN_PATH", pluginDir)
	t.Setenv("PATH", t.TempDir())

	code, stdout, stderr := runCLI("version")
	if code != 0 || stderr != "" || !strings.HasPrefix(stdout, "norns ") || strings.Contains(stdout, "extension override") {
		t.Fatalf("core command was overridden: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}
