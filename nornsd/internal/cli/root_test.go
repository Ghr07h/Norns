package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Ghr07h/norns/nornsd/internal/config"
	"github.com/Ghr07h/norns/nornsd/internal/domain"
	"github.com/Ghr07h/norns/nornsd/internal/version"
)

func TestVersionCommandUsesManifest(t *testing.T) {
	var output bytes.Buffer
	command := versionCommand(&output)
	command.SetArgs([]string{"--json"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	var info struct {
		Version            string         `json:"version"`
		DatabaseSchema     int            `json:"database_schema"`
		APISchema          int            `json:"api_schema"`
		AgentConfigVersion int            `json:"agent_config_version"`
		NornsProtocol      int            `json:"norns_protocol"`
		Plugins            map[string]int `json:"plugins"`
	}
	if err := json.Unmarshal(output.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info.Version != version.ProductVersion || info.DatabaseSchema != version.DatabaseSchema || info.APISchema != version.APISchema || info.AgentConfigVersion != version.AgentConfigVersion || info.NornsProtocol != version.NornsProtocol || info.Plugins["cc"] != version.CCPluginVersion {
		t.Fatalf("version output does not match versions.json: %s", output.String())
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(output.Bytes(), &fields); err != nil {
		t.Fatal(err)
	}
	if _, exists := fields["api_version"]; exists {
		t.Fatal("version output must use api_schema instead of api_version")
	}
	output.Reset()
	command = versionCommand(&output)
	command.SetArgs([]string{})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "API schema "+strconv.Itoa(version.APISchema)) {
		t.Fatalf("human version output does not report the API schema: %s", output.String())
	}
}

func TestNewInteractiveSubjectHasDisplayName(t *testing.T) {
	now := time.Date(2026, time.October, 16, 8, 30, 0, 0, time.UTC)
	subject := newInteractiveSubject(now, true)

	if subject.Name == nil || *subject.Name != interactiveSubjectName {
		t.Fatalf("subject.Name = %v, want %q", subject.Name, interactiveSubjectName)
	}
	if subject.ID == "" || !strings.HasPrefix(subject.DirectoryName, subject.ID+"_") {
		t.Fatalf("invalid identity: %#v", subject)
	}
	if !subject.CreatedAt.Equal(now) || !subject.UpdatedAt.Equal(now) {
		t.Fatalf("subject timestamps = %v, %v; want %v", subject.CreatedAt, subject.UpdatedAt, now)
	}
}

func TestConfigFileIsRequired(t *testing.T) {
	for _, args := range [][]string{{"server"}, {"agent"}} {
		var out, errout bytes.Buffer
		if Execute(args, strings.NewReader(""), &out, &errout) != 2 || !strings.Contains(errout.String(), "INVALID_SETTING") {
			t.Fatalf("args=%v stderr=%s", args, errout.String())
		}
	}
}

func TestFirstServiceConfigIsAuthoritative(t *testing.T) {
	first := config.Config{ServiceRoot: "/first", ServiceConcurrency: 2, NornsServerHost: "127.0.0.1", NornsServerPort: 28081, NornsServerToken: "first"}
	second := config.Config{ServiceRoot: "/second", ServiceConcurrency: 1, NornsServerToken: "second", LLMModel: "different-model"}
	second.UseService(first)
	if second.ServiceRoot != "/first" || second.ServiceConcurrency != 2 || second.NornsServerToken != "first" || second.LLMModel != "different-model" {
		t.Fatalf("service/agent separation failed: %#v", second)
	}
}

func TestServerCommandUsesOnlyConfigs(t *testing.T) {
	cmd := serverCommand(&bytes.Buffer{}, &bytes.Buffer{})
	if cmd.Flags().Lookup("configs") == nil {
		t.Fatal("missing --configs")
	}
	for _, removed := range []string{"host", "port", "token", "workspace-root", "norns", "agent", "concurrency", "settings"} {
		if cmd.Flags().Lookup(removed) != nil {
			t.Fatalf("obsolete flag %s", removed)
		}
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
