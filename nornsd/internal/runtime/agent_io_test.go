//go:build !windows

package runtime_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Ghr07h/norns/nornsd/internal/app"
	"github.com/Ghr07h/norns/nornsd/internal/config"
	"github.com/Ghr07h/norns/nornsd/internal/domain"
	"github.com/Ghr07h/norns/nornsd/internal/root"
	runtimemanager "github.com/Ghr07h/norns/nornsd/internal/runtime"
	"github.com/google/uuid"
)

func managedFixture(t *testing.T, body string) (*app.Service, *root.Owner) {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "agent.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"+body), 0700); err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(dir, "nornsd.yaml")
	data, _ := json.Marshal(map[string]any{"AGENT_LLM_URL": "https://example.test", "AGENT_LLM_KEY": "fixture", "AGENT_LLM_MODEL": "fixture", "AGENT_EXEC": []string{"/bin/sh", script}})
	if err := os.WriteFile(settings, data, 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(settings)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	owner, err := root.Acquire(ctx, filepath.Join(dir, "root"), "test", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	manager := runtimemanager.NewManager(ctx, runtimemanager.Options{Owner: owner, Concurrency: 1, SlotConfigs: []*config.Config{nil, &cfg}})
	manager.Start()
	t.Cleanup(func() { manager.Close(); _ = owner.Close() })
	return &app.Service{Owner: owner, Store: owner.Store, Manager: manager}, owner
}

const replyInitialize = `
 *'"type":"control_request"'*)
 id=$(printf '%s' "$line" | sed -n 's/.*"request_id":"\([^"]*\)".*/\1/p')
 printf '{"type":"control_response","response":{"request_id":"%s","subtype":"success","response":{}}}\n' "$id";;
`
const echoUser = `printf '%s\n' "$line" | sed 's/"type":"user"/"type":"user","isReplay":true/'`

func TestManagedMergedInputsIncludingNotificationOrigin(t *testing.T) {
	for _, notification := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "notification"}[notification], func(t *testing.T) {
			initial := ""
			origin := ""
			if notification {
				initial = `if [ "$count" -eq 1 ]; then printf '{"type":"result","uuid":"initial","is_error":false}\n{"type":"system","subtype":"status","status":"requesting"}\n'; fi`
				origin = `,"origin":{"kind":"task-notification"}`
			}
			body := `count=0
while IFS= read -r line; do
 case "$line" in
` + replyInitialize + `
 *'"type":"user"'*)
 count=$((count+1))
 ` + echoUser + `
 ` + initial + `
 if [ "$count" -eq 3 ]; then printf '{"type":"result","uuid":"merged","is_error":false,"num_turns":2` + origin + `,"usage":{"input_tokens":9}}\n'; fi
 ;;
 esac
done
`
			service, owner := managedFixture(t, body)
			ctx := context.Background()
			instruction := "first"
			subject, err := service.CreateSubject(ctx, nil, &instruction)
			if err != nil {
				t.Fatal(err)
			}
			var rid string
			waitFor(t, 5*time.Second, func() bool {
				s, err := owner.Store.GetSubject(ctx, subject.ID)
				if err != nil || s.CurrentRuntimeID == nil {
					return false
				}
				rid = *s.CurrentRuntimeID
				events, _ := owner.Store.ListEvents(ctx, subject.ID, 100, nil, nil)
				for _, e := range events {
					if (!notification && e.Type == "instruction.accepted") || (notification && e.Type == "agent.status") {
						return true
					}
				}
				return false
			})
			for _, text := range []string{"second", "third"} {
				if _, _, err = service.Submit(ctx, subject.ID, text, false); err != nil {
					t.Fatal(err)
				}
			}
			waitFor(t, 5*time.Second, func() bool {
				r, err := owner.Store.GetRuntime(ctx, subject.ID, rid, false)
				return err == nil && r.State == domain.RuntimeWaiting && r.InflightInputs == 0
			})
			events, err := owner.Store.ListEvents(ctx, subject.ID, 100, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			var settled []uint64
			for _, e := range events {
				if e.Type == "instruction.settled" {
					var p struct {
						Sequences []uint64 `json:"instruction_sequences"`
					}
					_ = json.Unmarshal(e.Payload, &p)
					settled = append(settled, p.Sequences...)
				}
			}
			if len(settled) != 3 {
				t.Fatalf("settled=%v", settled)
			}
		})
	}
}

func TestManagedAPIErrorsAndInterruptAllowNextInstruction(t *testing.T) {
	body := `count=0
while IFS= read -r line; do
 case "$line" in
` + replyInitialize + `
 *'"type":"user"'*)
 count=$((count+1))
 ` + echoUser + `
 case "$count" in
 1) printf '{"type":"system","subtype":"api_retry","attempt":1,"max_retries":10,"retry_delay_ms":500,"error_status":429,"error":"rate_limit"}\n{"type":"assistant","error":"rate_limit","message":{"model":"<synthetic>","content":[{"type":"text","text":"API Error: 429"}]}}\n{"type":"result","uuid":"rate-error","subtype":"success","is_error":true,"api_error_status":429,"terminal_reason":"completed"}\n';;
 2) printf '{"type":"result","uuid":"interrupt-error","subtype":"error_during_execution","is_error":true,"terminal_reason":"aborted_streaming","errors":["interrupted"]}\n';;
 3) printf '{"type":"result","uuid":"recovered","subtype":"success","is_error":false}\n';;
 esac;;
 esac
done
`
	service, owner := managedFixture(t, body)
	ctx := context.Background()
	subject, err := service.CreateSubject(ctx, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var rid string
	for i, text := range []string{"fail-api", "interrupt", "recover"} {
		r, _, err := service.Submit(ctx, subject.ID, text, false)
		if err != nil {
			t.Fatal(err)
		}
		if i > 0 && r.ID != rid {
			t.Fatal("error unexpectedly ended Runtime")
		}
		rid = r.ID
		waitFor(t, 5*time.Second, func() bool {
			current, err := owner.Store.GetRuntime(ctx, subject.ID, rid, false)
			return err == nil && current.State == domain.RuntimeWaiting && strings.Contains(string(current.UsageJSON), `"result_count":`+strconv.Itoa(i+1))
		})
	}
	r, err := owner.Store.GetRuntime(ctx, subject.ID, rid, false)
	if err != nil || r.ErrorCode != nil || !strings.Contains(string(r.UsageJSON), `"error_result_count":2`) {
		t.Fatalf("runtime=%+v err=%v", r, err)
	}
}

func TestMissingResumeSessionKeepsStartupError(t *testing.T) {
	service, owner := managedFixture(t, `while IFS= read -r line; do
 printf '{"type":"result","subtype":"error_during_execution","is_error":true,"num_turns":0,"errors":["No conversation found with session ID: missing"]}\n'
 printf 'No conversation found with session ID: missing\n' >&2
 exit 1
done
`)
	ctx := context.Background()
	subject, err := service.CreateSubject(ctx, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	previous := uuid.NewString()
	if _, err = owner.Store.CreateDirectRuntime(ctx, subject.ID, previous, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err = owner.Store.SetRuntimeTerminal(ctx, subject.ID, previous, domain.RuntimeEnded, "completed", "", time.Now(), true); err != nil {
		t.Fatal(err)
	}
	r, _, err := service.Submit(ctx, subject.ID, "continue", true)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, func() bool {
		s, err := owner.Store.GetSubject(ctx, subject.ID)
		return err == nil && s.State == domain.SubjectIdle
	})
	r, err = owner.Store.GetRuntime(ctx, subject.ID, r.ID, false)
	if err != nil || r.ErrorCode == nil || *r.ErrorCode != "AGENT_STARTUP_FAILED" || !strings.Contains(string(r.ErrorJSON), "No conversation found") {
		t.Fatalf("startup error=%+v err=%v", r, err)
	}
	events, err := owner.Store.ListEvents(ctx, subject.ID, 100, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range events {
		if e.Type == "runtime.session_resumed" {
			t.Fatal("missing session reported resumed")
		}
		if e.Type == "agent.result" && strings.Contains(string(e.Payload), "No conversation found") {
			found = true
		}
	}
	if !found {
		t.Fatal("startup result was swallowed")
	}
}
