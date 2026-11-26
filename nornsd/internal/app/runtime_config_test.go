package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Ghr07h/norns/nornsd/internal/domain"
	"github.com/Ghr07h/norns/nornsd/internal/root"
	runtimemanager "github.com/Ghr07h/norns/nornsd/internal/runtime"
)

func TestQueuedRuntimeCannotChangeConfigOnAppend(t *testing.T) {
	ctx := context.Background()
	owner, err := root.Acquire(ctx, t.TempDir(), "test", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	manager := runtimemanager.NewManager(ctx, runtimemanager.Options{Owner: owner, Concurrency: 1})
	defer manager.Close()
	service := &Service{Owner: owner, Store: owner.Store, Manager: manager}
	instruction := "first"
	subject, err := service.CreateSubject(ctx, nil, &instruction, json.RawMessage(`{"AGENT_LLM_MODEL":"created-model"}`))
	if err != nil {
		t.Fatal(err)
	}
	current, err := service.GetCurrentRuntime(ctx, subject.ID)
	if err != nil {
		t.Fatal(err)
	}
	rid := current.Runtime.ID
	if _, _, err = service.Submit(ctx, subject.ID, "must not append", false, json.RawMessage(`{"AGENT_LLM_MODEL":"other"}`)); domain.AsError(err).Code != "RUNTIME_CONFIG_CONFLICT" {
		t.Fatal(err)
	}
	r, err := owner.Store.GetRuntime(ctx, subject.ID, rid, true)
	if err != nil || len(r.Instructions) != 1 || string(r.ConfigOverride) != `{"AGENT_LLM_MODEL":"created-model"}` {
		t.Fatalf("override changed queue: %+v %v", r, err)
	}
	for _, raw := range []string{`null`, `{}`, ` { } `} {
		if _, _, err = service.Submit(ctx, subject.ID, "append", false, json.RawMessage(raw)); err != nil {
			t.Fatal(err)
		}
	}
	r, err = owner.Store.GetRuntime(ctx, subject.ID, rid, true)
	if err != nil || len(r.Instructions) != 4 {
		t.Fatalf("empty overrides changed append behavior: %+v %v", r, err)
	}
}
