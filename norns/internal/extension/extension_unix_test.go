//go:build !windows

package extension

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Ghr07h/norns/norns/internal/domain"
	"github.com/Ghr07h/norns/norns/internal/version"
)

func TestFindAndRunExtension(t *testing.T) {
	pluginDir := t.TempDir()
	pathDir := t.TempDir()
	workspace := t.TempDir()
	plugin := filepath.Join(pluginDir, "norns-websearch")
	pathPlugin := filepath.Join(pathDir, "norns-websearch")
	script := `#!/bin/sh
printf '%s\n' "$NORNS_PROTOCOL_VERSION|$NORNS_SCHEMA_VERSION|$NORNS_WORKSPACE|$PWD|$*"
`
	if err := os.WriteFile(plugin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pathPlugin, []byte("#!/bin/sh\nexit 99\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NORNS_PLUGIN_PATH", pluginDir)
	t.Setenv("PATH", pathDir)
	found, err := findInDirectory(pluginDir, "websearch")
	if err != nil {
		t.Fatal(err)
	}
	if found != plugin {
		t.Fatalf("sibling extension was not selected: %s", found)
	}
	var stdout, stderr bytes.Buffer
	if err := Run(found, []string{"--query", "sqlite wal"}, strings.NewReader(""), &stdout, &stderr, workspace); err != nil {
		t.Fatal(err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("extension wrote stderr: %s", stderr.String())
	}
	parts := strings.Split(strings.TrimSpace(stdout.String()), "|")
	resolvedWorkspace, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 5 || parts[0] != strconv.Itoa(version.ProtocolVersion) || parts[1] != strconv.Itoa(version.SchemaVersion) || parts[2] != workspace || parts[3] != resolvedWorkspace || parts[4] != "--query sqlite wal" {
		t.Fatalf("extension environment or arguments are wrong: %q", stdout.String())
	}
}

func TestFindExtensionErrors(t *testing.T) {
	t.Setenv("NORNS_PLUGIN_PATH", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	if _, err := Find("../bad"); errorCode(err) != "INVALID_ARGUMENT" {
		t.Fatalf("invalid name returned %v", err)
	}
	if _, err := Find("missing"); errorCode(err) != "EXTENSION_NOT_FOUND" {
		t.Fatalf("missing extension returned %v", err)
	}
}

func TestListSkipsNonExecutableFilesAndBrokenSymlinks(t *testing.T) {
	directory := t.TempDir()
	pathDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "norns-list-fallback"), []byte("not executable"), 0o600); err != nil {
		t.Fatal(err)
	}
	fallback := writeExtensionFixture(t, pathDir, "list-fallback")
	if err := os.WriteFile(filepath.Join(directory, "norns-list-unavailable"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(directory, "missing"), filepath.Join(directory, "norns-list-broken")); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(directory, "norns-list-linked")
	if err := os.Symlink(fallback, linked); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NORNS_PLUGIN_PATH", directory)
	t.Setenv("PATH", pathDir)
	items, err := listDirectory(directory)
	if err != nil || len(items) != 1 || items[0] != (Info{Name: "list-linked", Path: linked}) {
		t.Fatalf("items=%+v err=%v", items, err)
	}
}

func errorCode(err error) string {
	if domainErr, ok := err.(*domain.Error); ok {
		return domainErr.Code
	}
	return ""
}
