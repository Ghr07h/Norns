//go:build !windows

package runtime

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	ccagent "github.com/Ghr07h/norns/nornsd/internal/agent/cc"
	"github.com/Ghr07h/norns/nornsd/internal/domain"
	"github.com/Ghr07h/norns/nornsd/internal/eventlog"
	"github.com/Ghr07h/norns/nornsd/internal/root"
	"github.com/google/uuid"
)

func TestNornsCloseSettlesMergedInputsInNotificationResult(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	now := time.Now().UTC()
	owner, err := root.Acquire(ctx, t.TempDir(), "test", now)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	manager := NewManager(ctx, Options{Owner: owner, Concurrency: 1})
	defer manager.Close()
	sid, rid := uuid.NewString(), uuid.NewString()
	subject := domain.Subject{ID: sid, DirectoryName: root.DirectoryName(now, sid)}
	if err = owner.Store.CreateSubject(ctx, subject, nil, "", now); err != nil {
		t.Fatal(err)
	}
	path, err := owner.MaterializeSubject(ctx, subject)
	if err != nil {
		t.Fatal(err)
	}
	r, err := owner.Store.CreateDirectRuntime(ctx, sid, rid, now)
	if err != nil {
		t.Fatal(err)
	}
	r.Profile = domain.RuntimeProfile{Agent: "cc", Capabilities: []string{"norns"}}
	session, err := ccagent.StartManaged(ctx, ccagent.Launch{Exec: []string{"/bin/sh", "-c", "cat >/dev/null"}, SessionID: rid}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Terminate(context.Background())
	mr := &managedRuntime{subject: subject, runtime: r, path: path, session: session, initialized: true, done: make(chan struct{}), wake: make(chan struct{}, 1), logger: eventlog.New(owner.Store, path, sid, rid)}
	defer close(mr.done)
	for range 3 {
		if _, err = owner.Store.SubmitInstruction(ctx, sid, uuid.NewString(), "work", false, now); err != nil {
			t.Fatal(err)
		}
		_, group, err := owner.Store.DispatchPending(ctx, sid, rid, false, now)
		if err != nil {
			t.Fatal(err)
		}
		if err = owner.Store.ConfirmDispatch(ctx, sid, rid, group, now); err != nil {
			t.Fatal(err)
		}
		frame, _ := json.Marshal(map[string]any{"type": "user", "isReplay": true, "uuid": domain.InputUUID(rid, group), "message": map[string]any{"content": "work"}})
		if err = manager.handleMessage(mr, frame); err != nil {
			t.Fatal(err)
		}
	}
	for _, frame := range []string{
		`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"close-tool","name":"Bash","input":{"command":"norns task close --outcome succeeded"}}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"close-tool","is_error":false,"content":"{\"ok\":true,\"data\":{\"id\":\"task-1\",\"outcome\":\"succeeded\"}}"}]}}`,
		`{"type":"result","uuid":"notification-final","origin":{"kind":"task-notification"},"is_error":false,"num_turns":2,"usage":{"input_tokens":7}}`,
	} {
		if err = manager.handleMessage(mr, json.RawMessage(frame)); err != nil {
			t.Fatal(err)
		}
	}
	r, err = owner.Store.GetRuntime(ctx, sid, rid, false)
	if err != nil || r.State != domain.RuntimeEnded || r.InflightInputs != 0 || !r.Closing {
		t.Fatalf("Norns close: %+v %v", r, err)
	}
	if _, err = session.Wait(); err != nil {
		t.Fatal(err)
	}
}
