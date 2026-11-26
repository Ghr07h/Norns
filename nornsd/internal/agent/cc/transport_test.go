//go:build !windows

package cc

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestWaitDrainsBothStreamsAfterProcessExit(t *testing.T) {
	script := filepath.Join(t.TempDir(), "frames.sh")
	if err := os.WriteFile(script, []byte(`#!/bin/sh
i=0
while [ "$i" -lt 2000 ]; do
 printf '{"type":"result","uuid":"frame-%d","usage":{"input_tokens":1}}\n' "$i"
 i=$((i+1))
done
printf 'stderr-final-frame' >&2
`), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var stdout, stderr atomic.Int32
	s, err := StartManaged(ctx, Launch{Exec: []string{"/bin/sh", script}, SessionID: uuid.NewString()}, func(stream, kind string, payload json.RawMessage) error {
		if stream == "stderr" && strings.Contains(string(payload), "stderr-final-frame") {
			stderr.Add(1)
		}
		return nil
	}, func(json.RawMessage) error { stdout.Add(1); return nil })
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.Wait()
	if err != nil || result.ExitCode != 0 || stdout.Load() != 2000 || stderr.Load() != 1 {
		t.Fatalf("result=%+v stdout=%d stderr=%d err=%v", result, stdout.Load(), stderr.Load(), err)
	}
	_ = s.CloseInput()
}

func TestUnsupportedReverseControlIsAnswered(t *testing.T) {
	script := filepath.Join(t.TempDir(), "reverse.sh")
	if err := os.WriteFile(script, []byte(`#!/bin/sh
printf '{"type":"control_request","request_id":"reverse-1","request":{"subtype":"can_use_tool"}}\n'
IFS= read -r response
case "$response" in
 *'"subtype":"error"'*'nornsd does not support'*|*'nornsd does not support'*'"subtype":"error"'*) exit 0;;
 *) exit 2;;
esac
`), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s, err := StartManaged(ctx, Launch{Exec: []string{"/bin/sh", script}, SessionID: uuid.NewString()}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.Wait()
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("reverse control: %+v %v", result, err)
	}
	_ = s.CloseInput()
}
