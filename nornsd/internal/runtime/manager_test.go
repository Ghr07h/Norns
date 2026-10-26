//go:build !windows

package runtime_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Ghr07h/norns/nornsd/internal/app"
	"github.com/Ghr07h/norns/nornsd/internal/config"
	"github.com/Ghr07h/norns/nornsd/internal/domain"
	"github.com/Ghr07h/norns/nornsd/internal/root"
	runtimemanager "github.com/Ghr07h/norns/nornsd/internal/runtime"
	"github.com/Ghr07h/norns/nornsd/internal/version"
)

func TestManagedRuntimeReachesWaitingAndStops(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-cc.sh")
	source := `#!/bin/sh
while IFS= read -r line; do
  case "$line" in
    *'"type":"control_request"'*)
      id=$(printf '%s' "$line" | sed -n 's/.*"request_id":"\([^"]*\)".*/\1/p')
      printf '{"type":"control_response","response":{"request_id":"%s","subtype":"success","response":{}}}\n' "$id"
      ;;
	*'"type":"user"'*)
	  printf '{"type":"assistant","message":{"content":[{"type":"text","text":"done"}]}}\n'
	  printf '{"type":"result","subtype":"success","session_id":"runtime-session","is_error":false,"num_turns":1,"usage":{"input_tokens":1},"result":"done"}\n'
	  printf '{"type":"result","subtype":"success","session_id":"runtime-session","is_error":false,"num_turns":1,"origin":{"kind":"task-notification"},"usage":{"input_tokens":12,"output_tokens":4},"result":"background task completed"}\n'
      ;;
  esac
done
`
	if err := os.WriteFile(script, []byte(source), 0700); err != nil {
		t.Fatal(err)
	}
	customAgents := filepath.Join(dir, "custom-agents")
	if err := os.MkdirAll(customAgents, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(customAgents, "reviewer.md"), []byte("---\nname: reviewer\ndescription: review\n---\nreview"), 0600); err != nil {
		t.Fatal(err)
	}
	customPluginOne := filepath.Join(dir, "custom-plugin-one")
	customPluginTwo := filepath.Join(dir, "custom-plugin-two")
	for _, pluginDir := range []string{customPluginOne, customPluginTwo} {
		if err := os.MkdirAll(pluginDir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	setting := filepath.Join(dir, "agent.yaml")
	yaml := "VERSION: 1\nLLM_URL: https://example.test\nLLM_KEY: secret\nLLM_MODEL: fake\nCUSTOM_AGENTS_DIR: " + customAgents + "\nCUSTOM_PLUGIN_DIRS:\n  - " + customPluginOne + "\n  - " + customPluginTwo + "\nAGENT_EXEC:\n  - /bin/sh\n  - " + script + "\n"
	if err := os.WriteFile(setting, []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(setting)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	owner, err := root.Acquire(ctx, filepath.Join(dir, "root"), "test", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	profile := domain.RuntimeProfile{Agent: "cc", AgentVersion: version.ClaudeCodeVersion}
	slots := make([]*config.Config, 2)
	slots[1] = &cfg
	manager := runtimemanager.NewManager(ctx, runtimemanager.Options{Owner: owner, Profile: profile, Concurrency: 1, SlotConfigs: slots, MemoryLimit: ^uint64(0)})
	manager.Start()
	defer func() { manager.Close(); _ = owner.Close() }()
	service := &app.Service{Owner: owner, Store: owner.Store, Manager: manager, Profile: profile}
	instruction := "do work"
	subject, err := service.CreateSubject(ctx, nil, &instruction)
	if err != nil {
		t.Fatal(err)
	}
	var runtimeID string
	waitFor(t, 5*time.Second, func() bool {
		s, current, err := service.GetSubject(ctx, subject.ID)
		if err != nil || s.State != domain.SubjectUsing || current.Runtime == nil {
			return false
		}
		runtimeID = current.Runtime.ID
		return current.Runtime.State == domain.RuntimeWaiting
	})
	subjectPath, err := owner.SubjectPath(subject.DirectoryName)
	if err != nil {
		t.Fatal(err)
	}
	runtimeRoot := filepath.Join(subjectPath, ".nornsd", "runtimes", runtimeID)
	memorySamplesPath := filepath.Join(runtimeRoot, "memory-samples.jsonl")
	waitFor(t, 5*time.Second, func() bool {
		data, readErr := os.ReadFile(memorySamplesPath)
		return readErr == nil && len(bytes.TrimSpace(data)) > 0
	})
	var events []domain.Event
	waitFor(t, 5*time.Second, func() bool {
		var listErr error
		events, listErr = owner.Store.ListEvents(ctx, subject.ID, 100, nil, nil)
		if listErr != nil {
			return false
		}
		count := 0
		for _, event := range events {
			if event.Type == "agent.result" {
				count++
			}
		}
		return count == 2
	})
	resultCount, notificationCount := 0, 0
	for _, event := range events {
		if event.Type != "agent.result" {
			continue
		}
		resultCount++
		var payload struct {
			OriginKind string `json:"origin_kind"`
			NumTurns   int    `json:"num_turns"`
			Result     string `json:"result"`
		}
		if json.Unmarshal(event.Payload, &payload) == nil && payload.OriginKind == "task-notification" {
			notificationCount++
			if payload.NumTurns != 1 || payload.Result != "background task completed" {
				t.Fatalf("task notification payload=%s", event.Payload)
			}
		}
	}
	if resultCount != 2 || notificationCount != 1 {
		t.Fatalf("agent result events=%d task notifications=%d: %#v", resultCount, notificationCount, events)
	}
	if _, err = service.Stop(ctx, subject.ID, "graceful"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, func() bool {
		s, _, err := service.GetSubject(ctx, subject.ID)
		return err == nil && s.State == domain.SubjectIdle
	})
	runtime, err := service.GetRuntime(ctx, subject.ID, runtimeID)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.State != domain.RuntimeCancelled {
		t.Fatalf("runtime state=%s", runtime.State)
	}
	var resourceData struct {
		SampleIntervalMS int64 `json:"memory_sample_interval_ms"`
	}
	if err = json.Unmarshal(runtime.ResourceJSON, &resourceData); err != nil || resourceData.SampleIntervalMS != 500 {
		t.Fatalf("runtime resource sample interval=%d, want 500: %s, %v", resourceData.SampleIntervalMS, runtime.ResourceJSON, err)
	}
	var processData struct {
		SessionID string `json:"session_id"`
	}
	if json.Unmarshal(runtime.ProcessJSON, &processData) != nil || processData.SessionID != runtimeID {
		t.Fatalf("runtime process session ID=%q, want %q: %s", processData.SessionID, runtimeID, runtime.ProcessJSON)
	}
	memoryData, err := os.ReadFile(memorySamplesPath)
	if err != nil {
		t.Fatal(err)
	}
	firstSample := bytes.Split(bytes.TrimSpace(memoryData), []byte("\n"))[0]
	var sample struct {
		SampledAt   time.Time `json:"sampled_at"`
		MemoryBytes uint64    `json:"memory_bytes"`
	}
	if err = json.Unmarshal(firstSample, &sample); err != nil || sample.SampledAt.IsZero() || sample.MemoryBytes == 0 {
		t.Fatalf("invalid memory sample %q: %#v, %v", firstSample, sample, err)
	}
	for _, name := range []string{"snapshot.json", "launch.json", "config.snapshot.json"} {
		data, readErr := os.ReadFile(filepath.Join(runtimeRoot, name))
		if readErr != nil {
			t.Fatalf("read %s: %v", name, readErr)
		}
		if bytes.Contains(data, []byte("secret")) || bytes.Contains(data, []byte(script)) {
			t.Fatalf("%s contains a credential or wrapper argument", name)
		}
	}
	launchData, err := os.ReadFile(filepath.Join(runtimeRoot, "launch.json"))
	if err != nil {
		t.Fatal(err)
	}
	var launchArtifact struct {
		Argv        []string `json:"argv"`
		PluginCount int      `json:"plugin_count"`
		SessionID   string   `json:"session_id"`
	}
	if err = json.Unmarshal(launchData, &launchArtifact); err != nil {
		t.Fatal(err)
	}
	customPlugin := filepath.Join(runtimeRoot, "artifacts", "custom-plugin")
	var loadedPluginDirs []string
	for i, argument := range launchArtifact.Argv {
		if argument == "--plugin-dir" && i+1 < len(launchArtifact.Argv) {
			loadedPluginDirs = append(loadedPluginDirs, launchArtifact.Argv[i+1])
		}
	}
	wantPluginDirs := []string{customPluginOne, customPluginTwo, customPlugin}
	if launchArtifact.PluginCount != len(wantPluginDirs) || !slices.Equal(loadedPluginDirs, wantPluginDirs) {
		t.Fatalf("loaded Plugin directories = %#v (count %d), want %#v", loadedPluginDirs, launchArtifact.PluginCount, wantPluginDirs)
	}
	if launchArtifact.SessionID != runtimeID || !slices.Contains(launchArtifact.Argv, "--session-id="+runtimeID) {
		t.Fatalf("launch session=%q argv=%#v, want Runtime ID %q", launchArtifact.SessionID, launchArtifact.Argv, runtimeID)
	}
	for _, argument := range launchArtifact.Argv {
		if argument == "--resume" || strings.HasPrefix(argument, "--resume=") {
			t.Fatalf("launch unexpectedly resumes a previous session: %#v", launchArtifact.Argv)
		}
	}
	for _, argument := range []string{"--dangerously-skip-permissions", "--setting-sources=", "--plugin-dir"} {
		if !bytes.Contains(launchData, []byte(argument)) {
			t.Fatalf("launch.json is missing %s: %s", argument, launchData)
		}
	}
	if bytes.Contains(launchData, []byte("--agent")) || bytes.Contains(launchData, []byte("--system-prompt")) {
		t.Fatalf("ordinary Runtime unexpectedly overrides the root agent or system prompt: %s", launchData)
	}
	for _, name := range []string{filepath.Join(".claude-plugin", "plugin.json"), filepath.Join("agents", "reviewer.md"), "plugin.manifest.json"} {
		if _, err = os.Stat(filepath.Join(customPlugin, name)); err != nil {
			t.Fatalf("custom plugin is missing %s: %v", name, err)
		}
	}
	if _, err = os.Stat(filepath.Join(subjectPath, ".claude")); !os.IsNotExist(err) {
		t.Fatalf("Runtime wrote Subject .claude directory: %v", err)
	}
	artifactConfig, err := os.ReadFile(filepath.Join(runtimeRoot, "artifacts", "config", "agent.yaml"))
	if err != nil || !bytes.Contains(artifactConfig, []byte("secret")) {
		t.Fatalf("restricted config artifact is incomplete: %v", err)
	}
}

func TestManagedResumeForkIgnoresReplayedStartupResult(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-cc-resume.sh")
	source := `#!/bin/sh
printf '{"type":"result","subtype":"success","is_error":false,"usage":{"input_tokens":0}}\n'
while IFS= read -r line; do
  case "$line" in
    *'"type":"control_request"'*)
	  printf '{"type":"system","subtype":"task_notification","task_id":"old-1","status":"failed"}\n'
	  printf '{"type":"system","subtype":"task_notification","task_id":"old-2","status":"failed"}\n'
	  printf '{"type":"system","subtype":"task_notification","task_id":"old-3","status":"failed"}\n'
      id=$(printf '%s' "$line" | sed -n 's/.*"request_id":"\([^"]*\)".*/\1/p')
      printf '{"type":"control_response","response":{"request_id":"%s","subtype":"success","response":{}}}\n' "$id"
      ;;
    *'"type":"user"'*)
	  printf '{"type":"system","subtype":"init"}\n'
	  printf '{"type":"result","subtype":"success","is_error":false,"num_turns":0,"origin":{"kind":"task-notification"},"usage":{"input_tokens":0,"output_tokens":0}}\n'
	  printf '{"type":"system","subtype":"init"}\n'
	  printf '{"type":"result","subtype":"success","is_error":false,"num_turns":0,"origin":{"kind":"task-notification"},"usage":{"input_tokens":0,"output_tokens":0}}\n'
	  printf '{"type":"system","subtype":"init"}\n'
	  printf '{"type":"result","subtype":"success","is_error":false,"num_turns":0,"origin":{"kind":"task-notification"},"usage":{"input_tokens":0,"output_tokens":0}}\n'
      printf '{"type":"result","subtype":"success","is_error":false,"usage":{"input_tokens":1}}\n'
      ;;
  esac
done
`
	if err := os.WriteFile(script, []byte(source), 0700); err != nil {
		t.Fatal(err)
	}
	setting := filepath.Join(dir, "agent.yaml")
	yaml := "VERSION: 1\nLLM_URL: https://example.test\nLLM_KEY: secret\nLLM_MODEL: fake\nAGENT_EXEC:\n  - /bin/sh\n  - " + script + "\n"
	if err := os.WriteFile(setting, []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(setting)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	owner, err := root.Acquire(ctx, filepath.Join(dir, "root"), "test", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	profile := domain.RuntimeProfile{Agent: "cc", AgentVersion: version.ClaudeCodeVersion}
	slots := make([]*config.Config, 2)
	slots[1] = &cfg
	manager := runtimemanager.NewManager(ctx, runtimemanager.Options{Owner: owner, Profile: profile, Concurrency: 1, SlotConfigs: slots, MemoryLimit: ^uint64(0)})
	defer func() { manager.Close(); _ = owner.Close() }()
	service := &app.Service{Owner: owner, Store: owner.Store, Manager: manager, Profile: profile}
	subject, err := service.CreateSubject(ctx, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	previousID := "00000000-0000-4000-8000-000000000001"
	if _, err = owner.Store.CreateDirectRuntime(ctx, subject.ID, previousID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err = owner.Store.SetRuntimeTerminal(ctx, subject.ID, previousID, domain.RuntimeEnded, "completed", "", time.Now().UTC(), true); err != nil {
		t.Fatal(err)
	}
	manager.Start()
	runtime, _, err := service.Submit(ctx, subject.ID, "continue", true)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, func() bool {
		current, getErr := owner.Store.GetRuntime(ctx, subject.ID, runtime.ID, false)
		return getErr == nil && current.State == domain.RuntimeWaiting
	})
	subjectPath, err := owner.SubjectPath(subject.DirectoryName)
	if err != nil {
		t.Fatal(err)
	}
	launchData, err := os.ReadFile(filepath.Join(subjectPath, ".nornsd", "runtimes", runtime.ID, "launch.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--resume=" + previousID, "--fork-session", "--session-id=" + runtime.ID} {
		if !bytes.Contains(launchData, []byte(want)) {
			t.Fatalf("launch.json is missing %q: %s", want, launchData)
		}
	}
	events, err := owner.Store.ListEvents(ctx, subject.ID, 100, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	resumedCount, resultCount := 0, 0
	for _, event := range events {
		switch event.Type {
		case "runtime.session_resumed":
			resumedCount++
			var payload struct {
				ResumeSessionID string `json:"resume_session_id"`
				SessionID       string `json:"session_id"`
				OriginKind      string `json:"origin_kind"`
			}
			if err = json.Unmarshal(event.Payload, &payload); err != nil || payload.ResumeSessionID != previousID || payload.SessionID != runtime.ID {
				t.Fatalf("runtime.session_resumed payload=%s parsed=%#v err=%v", event.Payload, payload, err)
			}
			if resumedCount > 1 && payload.OriginKind != "task-notification" {
				t.Fatalf("delayed replay result has origin_kind=%q: %s", payload.OriginKind, event.Payload)
			}
		case "agent.result":
			resultCount++
		}
	}
	if resumedCount != 4 || resultCount != 1 {
		t.Fatalf("event counts: runtime.session_resumed=%d agent.result=%d; events=%#v", resumedCount, resultCount, events)
	}
	current, err := owner.Store.GetRuntime(ctx, subject.ID, runtime.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if current.State != domain.RuntimeWaiting || current.InflightTurns != 0 || current.ErrorCode != nil {
		t.Fatalf("runtime after replay notifications=%#v", current)
	}
	if _, err = service.Stop(ctx, subject.ID, "force"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, func() bool {
		current, getErr := owner.Store.GetRuntime(ctx, subject.ID, runtime.ID, false)
		return getErr == nil && current.State.Terminal()
	})
}

func TestManagerCloseWaitsForRuntimeFinalization(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-cc-close.sh")
	source := `#!/bin/sh
while IFS= read -r line; do
  case "$line" in
    *'"type":"control_request"'*)
      id=$(printf '%s' "$line" | sed -n 's/.*"request_id":"\([^"]*\)".*/\1/p')
      printf '{"type":"control_response","response":{"request_id":"%s","subtype":"success","response":{}}}\n' "$id"
      ;;
    *'"type":"user"'*)
      printf '{"type":"result","subtype":"success","is_error":false,"usage":{"input_tokens":1}}\n'
      ;;
  esac
done
`
	if err := os.WriteFile(script, []byte(source), 0700); err != nil {
		t.Fatal(err)
	}
	setting := filepath.Join(dir, "agent-close.yaml")
	yaml := "VERSION: 1\nLLM_URL: https://example.test\nLLM_KEY: secret\nLLM_MODEL: fake\nAGENT_EXEC:\n  - /bin/sh\n  - " + script + "\n"
	if err := os.WriteFile(setting, []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(setting)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	owner, err := root.Acquire(ctx, filepath.Join(dir, "root"), "test", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	profile := domain.RuntimeProfile{Agent: "cc", AgentVersion: version.ClaudeCodeVersion}
	slots := make([]*config.Config, 2)
	slots[1] = &cfg
	manager := runtimemanager.NewManager(ctx, runtimemanager.Options{Owner: owner, Profile: profile, Concurrency: 1, SlotConfigs: slots})
	manager.Start()
	defer manager.Close()
	service := &app.Service{Owner: owner, Store: owner.Store, Manager: manager, Profile: profile}
	instruction := "wait for shutdown"
	subject, err := service.CreateSubject(ctx, nil, &instruction)
	if err != nil {
		t.Fatal(err)
	}

	var runtimeID string
	waitFor(t, 5*time.Second, func() bool {
		_, current, getErr := service.GetSubject(ctx, subject.ID)
		if getErr != nil || current.Runtime == nil {
			return false
		}
		runtimeID = current.Runtime.ID
		return current.Runtime.State == domain.RuntimeWaiting
	})

	manager.Close()
	closedSubject, current, err := service.GetSubject(ctx, subject.ID)
	if err != nil {
		t.Fatal(err)
	}
	if closedSubject.State != domain.SubjectIdle || current.Runtime != nil {
		t.Fatalf("manager close returned before subject finalization: subject=%#v current=%#v", closedSubject, current)
	}
	runtime, err := service.GetRuntime(ctx, subject.ID, runtimeID)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.State != domain.RuntimeCancelled {
		t.Fatalf("runtime state after manager close=%s, want %s", runtime.State, domain.RuntimeCancelled)
	}
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}
