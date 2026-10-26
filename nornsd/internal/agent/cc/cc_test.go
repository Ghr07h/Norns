package cc

import (
	"slices"
	"strings"
	"testing"
)

func TestBuildArgsAlwaysSkipsPermissionPrompts(t *testing.T) {
	for _, managed := range []bool{false, true} {
		argv, err := BuildArgs(Launch{Exec: []string{"claude"}, SessionID: "runtime-1"}, managed)
		if err != nil {
			t.Fatalf("BuildArgs(managed=%v): %v", managed, err)
		}
		if !slices.Contains(argv, "--dangerously-skip-permissions") {
			t.Fatalf("BuildArgs(managed=%v) = %q, missing permission bypass", managed, argv)
		}
	}
}

func TestRequiresExactVersionForDirectClaudeExecutables(t *testing.T) {
	tests := []struct {
		name     string
		argv     []string
		explicit bool
		want     bool
	}{
		{name: "default", argv: []string{"claude"}, want: true},
		{name: "explicit name", argv: []string{"claude"}, explicit: true, want: true},
		{name: "explicit absolute path", argv: []string{"/cc/node_modules/.bin/claude"}, explicit: true, want: true},
		{name: "explicit windows executable", argv: []string{`C:\tools\claude.exe`}, explicit: true, want: true},
		{name: "wrapper", argv: []string{"cfuse", "--cc"}, explicit: true},
		{name: "test double", argv: []string{"/tmp/fake-cc.sh"}, explicit: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RequiresExactVersion(tt.argv, tt.explicit); got != tt.want {
				t.Fatalf("RequiresExactVersion(%q, %v)=%v, want %v", tt.argv, tt.explicit, got, tt.want)
			}
		})
	}
}

func TestBuildArgsRequiresSessionID(t *testing.T) {
	if _, err := BuildArgs(Launch{Exec: []string{"claude"}}, true); err == nil {
		t.Fatal("BuildArgs accepted an empty session ID")
	}
}

func TestBuildArgsForksResumedSessionIntoRuntimeSession(t *testing.T) {
	argv, err := BuildArgs(Launch{Exec: []string{"claude"}, SessionID: "runtime-2", ResumeSessionID: "runtime-1"}, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--resume=runtime-1", "--fork-session", "--session-id=runtime-2"} {
		if !slices.Contains(argv, want) {
			t.Fatalf("BuildArgs() = %q, missing %q", argv, want)
		}
	}
	if _, err = BuildArgs(Launch{Exec: []string{"claude"}, SessionID: "same", ResumeSessionID: "same"}, true); err == nil {
		t.Fatal("BuildArgs accepted identical resumed and new session IDs")
	}
}

func TestBuildArgsIsolatesOnlyManagedOrExplicitSettings(t *testing.T) {
	tests := []struct {
		name         string
		launch       Launch
		managed      bool
		wantIsolated bool
	}{
		{name: "interactive default settings", launch: Launch{Exec: []string{"claude"}, SessionID: "runtime-1"}},
		{name: "interactive explicit settings", launch: Launch{Exec: []string{"claude"}, SessionID: "runtime-1", SettingsPath: "/tmp/settings.json"}, wantIsolated: true},
		{name: "managed default settings", launch: Launch{Exec: []string{"claude"}, SessionID: "runtime-1"}, managed: true, wantIsolated: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			argv, err := BuildArgs(tt.launch, tt.managed)
			if err != nil {
				t.Fatal(err)
			}
			if got := slices.Contains(argv, "--setting-sources="); got != tt.wantIsolated {
				t.Fatalf("BuildArgs() = %q, settings isolation = %v, want %v", argv, got, tt.wantIsolated)
			}
		})
	}
}

func TestBuildArgsAddsMultiplePluginsAndNornsRootAgent(t *testing.T) {
	argv, err := BuildArgs(Launch{
		Exec:        []string{"cfuse", "--cc"},
		SessionID:   "runtime-1",
		PluginPaths: []string{"/tmp/custom", "/tmp/norns"},
		RootAgent:   "norns:coordinator",
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"cfuse", "--cc", "--dangerously-skip-permissions",
		"--output-format", "stream-json", "--verbose", "--input-format", "stream-json", "--include-partial-messages",
		"--session-id=runtime-1", "--setting-sources=",
		"--plugin-dir", "/tmp/custom", "--plugin-dir", "/tmp/norns",
		"--agent", "norns:coordinator",
	}
	if !slices.Equal(argv, want) {
		t.Fatalf("BuildArgs() = %q, want %q", argv, want)
	}
}

func TestBuildArgsRejectsPermissionBypassPassthrough(t *testing.T) {
	_, err := BuildArgs(Launch{
		Exec:        []string{"claude"},
		Passthrough: []string{"--dangerously-skip-permissions"},
		SessionID:   "runtime-1",
	}, false)
	if err == nil {
		t.Fatal("BuildArgs accepted nornsd-controlled permission bypass")
	}
}

func TestBuildArgsRejectsControlledNornsArguments(t *testing.T) {
	for _, argument := range []string{"--setting-sources=project", "--plugin-dir=/tmp/other", "--agent=other", "--fork-session"} {
		_, err := BuildArgs(Launch{
			Exec:        []string{"claude"},
			Passthrough: []string{argument},
			SessionID:   "runtime-1",
			PluginPaths: []string{"/tmp/norns"},
			RootAgent:   "norns:coordinator",
		}, false)
		if err == nil {
			t.Fatalf("BuildArgs accepted nornsd-controlled argument %s", argument)
		}
	}
}

func TestLaunchEnvironmentForcesSandbox(t *testing.T) {
	t.Setenv("IS_SANDBOX", "0")
	env := launchEnvironment([]string{"OTHER=value", "IS_SANDBOX=also-not-sandboxed"})
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], "IS_SANDBOX=") {
			if env[i] != sandboxEnvironment {
				t.Fatalf("effective sandbox environment = %q, want %q", env[i], sandboxEnvironment)
			}
			return
		}
	}
	t.Fatal("launch environment is missing IS_SANDBOX")
}
