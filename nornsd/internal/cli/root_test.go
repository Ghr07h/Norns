package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Ghr07h/norns/nornsd/internal/config"
	"github.com/Ghr07h/norns/nornsd/internal/domain"
)

func TestNewInteractiveSubjectHasDisplayName(t *testing.T) {
	now := time.Date(2026, time.October, 16, 8, 30, 0, 0, time.UTC)
	subject := newInteractiveSubject(now, true)

	if subject.Name == nil || *subject.Name != interactiveSubjectName {
		t.Fatalf("subject.Name = %v, want %q", subject.Name, interactiveSubjectName)
	}
	if subject.ID == "" || subject.SessionID != subject.ID {
		t.Fatalf("subject identity = ID %q, SessionID %q", subject.ID, subject.SessionID)
	}
	if !subject.Profile.Has("norns") {
		t.Fatalf("subject profile = %#v, want norns capability", subject.Profile)
	}
	if !subject.CreatedAt.Equal(now) || !subject.UpdatedAt.Equal(now) {
		t.Fatalf("subject timestamps = %v, %v; want %v", subject.CreatedAt, subject.UpdatedAt, now)
	}
}

func TestWaitForAutoSettingLaunchReportsSourcesAndCanCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stderr bytes.Buffer
	err := waitForAutoSettingLaunch(ctx, &stderr, []string{"/opt/nornsd/agent.yaml", "/data/subject/.nornsd/config/agent.yaml"})
	if !errors.Is(err, errLaunchCancelled) {
		t.Fatalf("waitForAutoSettingLaunch error = %v, want launch cancellation", err)
	}
	text := stderr.String()
	for _, want := range []string{"后者覆盖前者", "/opt/nornsd/agent.yaml", "/data/subject/.nornsd/config/agent.yaml", "3 秒后启动 Claude Code", "已取消启动"} {
		if !strings.Contains(text, want) {
			t.Fatalf("stderr does not contain %q:\n%s", want, text)
		}
	}
}

func TestServerOptionsUseFirstAgentSettingAsAuthority(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	content := "VERSION: 1\nLLM_URL: x\nLLM_KEY: y\nLLM_MODEL: z\nNORNS_ENABLE: false\nNORNS_SERVER_HOST: 0.0.0.0\nNORNS_SERVER_PORT: 29000\nNORNS_SERVER_TOKEN: yaml-token\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	got := (serverOptions{host: "127.0.0.1", port: 28081, token: "cli-token", useNorns: true}).withConfig(cfg)
	if got.host != "0.0.0.0" || got.port != 29000 || got.token != "yaml-token" || got.useNorns {
		t.Fatalf("server options did not use agent.yaml values: %#v", got)
	}
}

func TestServerCommandExposesTokenFlagOnly(t *testing.T) {
	cmd := serverCommand(&bytes.Buffer{}, &bytes.Buffer{})
	if cmd.Flags().Lookup("token") == nil {
		t.Fatal("server command does not expose --token")
	}
	if cmd.Flags().Lookup("password") != nil {
		t.Fatal("server command still exposes --password")
	}
}

func TestAgentCommandUsesResumeSubjectFlag(t *testing.T) {
	cmd := agentCommand(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	if cmd.Flags().Lookup("resume-subject") == nil {
		t.Fatal("agent command does not expose --resume-subject")
	}
	if cmd.Flags().Lookup("resume") != nil {
		t.Fatal("agent command still exposes --resume")
	}
	if cmd.Flags().Lookup("resume-session") == nil {
		t.Fatal("agent command does not expose --resume-session")
	}
}

func TestResumeSessionRequiresResumeSubject(t *testing.T) {
	cmd := agentCommand(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	cmd.SetArgs([]string{"--resume-session"})
	err := cmd.Execute()
	if err == nil || domain.AsError(err).Code != "INVALID_REQUEST" || !strings.Contains(err.Error(), "--resume-subject") {
		t.Fatalf("agent --resume-session error = %v", err)
	}
}

func TestRemovedPasswordFlagIsAnInvalidRequest(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Execute([]string{"server", "--password", "obsolete"}, strings.NewReader(""), &stdout, &stderr); code != 2 {
		t.Fatalf("exit code = %d, want 2; stderr=%s", code, stderr.String())
	}
	for _, want := range []string{`"code":"INVALID_REQUEST"`, "unknown flag: --password"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr does not contain %q: %s", want, stderr.String())
		}
	}
}

func TestWriteErrorIncludesWrappedCause(t *testing.T) {
	var stderr bytes.Buffer
	err := domain.Wrap("INVALID_SETTING", "cannot load agent setting", 400, errors.New("invalid setting: unknown field TEST"))
	if code := writeError(&stderr, err); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	for _, want := range []string{`"code":"INVALID_SETTING"`, `"cause":"invalid setting: unknown field TEST"`} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr does not contain %q: %s", want, stderr.String())
		}
	}
}
