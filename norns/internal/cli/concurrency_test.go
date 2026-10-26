package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestCLIHelperProcess(t *testing.T) {
	if os.Getenv("NORNS_TEST_HELPER") != "1" {
		return
	}
	workspace := os.Getenv("NORNS_TEST_WORKSPACE")
	if err := os.Chdir(workspace); err != nil {
		os.Exit(125)
	}
	separator := 0
	for index, arg := range os.Args {
		if arg == "--" {
			separator = index + 1
			break
		}
	}
	os.Exit(Execute(os.Args[separator:], os.Stdin, os.Stdout, os.Stderr))
}

func TestMultiProcessStateCompetition(t *testing.T) {
	workspace := t.TempDir()
	openArgs := [][]string{
		{"task", "open", "--purpose", "First", "--concurrency", "2", "--info", "Subject"},
		{"task", "open", "--purpose", "Second", "--concurrency", "2", "--info", "Subject"},
	}
	opened := runConcurrent(t, workspace, openArgs)
	if successes(opened) != 1 {
		t.Fatalf("exactly one concurrent task open must succeed: %#v", opened)
	}
	for _, result := range opened {
		if result.code != 0 && errorEnvelopeCode(result.stderr) != "OPEN_TASK_EXISTS" {
			t.Fatalf("losing task open returned an unexpected error: %#v", result)
		}
	}

	added := runHelper(t, workspace, "exploration", "add", "--exploration-title", "Concurrent", "--exploration-content", "Assign once")
	if added.code != 0 {
		t.Fatalf("cannot add Exploration: %#v", added)
	}
	explorationID := successEnvelopeID(t, added.stdout)
	assigned := runConcurrent(t, workspace, [][]string{
		{"exploration", "assign", "--exploration", explorationID, "--explorer", "one", "--output", "output://one"},
		{"exploration", "assign", "--exploration", explorationID, "--explorer", "two", "--output", "output://two"},
	})
	if successes(assigned) != 1 {
		t.Fatalf("exactly one concurrent assign must succeed: %#v", assigned)
	}
	for _, result := range assigned {
		if result.code != 0 && errorEnvelopeCode(result.stderr) != "INVALID_STATE" {
			t.Fatalf("losing assign returned an unexpected error: %#v", result)
		}
	}
	finished := runConcurrent(t, workspace, [][]string{
		{"exploration", "finish", "--exploration", explorationID, "--outcome", "succeeded", "--outcome-reason", "one"},
		{"exploration", "finish", "--exploration", explorationID, "--outcome", "failed", "--outcome-reason", "two"},
	})
	if successes(finished) != 1 {
		t.Fatalf("exactly one concurrent finish must succeed: %#v", finished)
	}
	for _, result := range finished {
		if result.code != 0 && errorEnvelopeCode(result.stderr) != "INVALID_STATE" {
			t.Fatalf("losing finish returned an unexpected error: %#v", result)
		}
	}

	firstWriter := runHelper(t, workspace, "exploration", "add", "--exploration-title", "Writer one", "--exploration-content", "Serialize one")
	secondWriter := runHelper(t, workspace, "exploration", "add", "--exploration-title", "Writer two", "--exploration-content", "Serialize two")
	if firstWriter.code != 0 || secondWriter.code != 0 {
		t.Fatalf("cannot prepare independent writers: %#v %#v", firstWriter, secondWriter)
	}
	independent := runConcurrent(t, workspace, [][]string{
		{"exploration", "assign", "--exploration", successEnvelopeID(t, firstWriter.stdout), "--explorer", "writer-one", "--output", "output://writer-one"},
		{"exploration", "assign", "--exploration", successEnvelopeID(t, secondWriter.stdout), "--explorer", "writer-two", "--output", "output://writer-two"},
	})
	if successes(independent) != 2 {
		t.Fatalf("serialized independent writers did not both succeed: %#v", independent)
	}

	readers := runConcurrent(t, workspace, [][]string{
		{"view", "status", "--json"},
		{"view", "status", "--json"},
		{"view", "status", "--json"},
		{"view", "status", "--json"},
	})
	if successes(readers) != len(readers) {
		t.Fatalf("concurrent Status readers failed: %#v", readers)
	}
	entries, err := os.ReadDir(filepath.Join(workspace, ".norns", "view"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(readers) {
		t.Fatalf("concurrent readers did not publish unique Views: %d", len(entries))
	}
}

type helperResult struct {
	code   int
	stdout string
	stderr string
}

func runConcurrent(t *testing.T, workspace string, calls [][]string) []helperResult {
	t.Helper()
	results := make([]helperResult, len(calls))
	var wait sync.WaitGroup
	start := make(chan struct{})
	for index := range calls {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			results[index] = runHelper(t, workspace, calls[index]...)
		}(index)
	}
	close(start)
	wait.Wait()
	return results
}

func runHelper(t *testing.T, workspace string, args ...string) helperResult {
	t.Helper()
	commandArgs := []string{"-test.run=^TestCLIHelperProcess$", "--"}
	commandArgs = append(commandArgs, args...)
	cmd := exec.Command(os.Args[0], commandArgs...)
	cmd.Env = append(os.Environ(), "NORNS_TEST_HELPER=1", "NORNS_TEST_WORKSPACE="+workspace)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()
	var err error
	select {
	case err = <-done:
	case <-time.After(20 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("helper process timed out: %v", args)
	}
	code := 0
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			code = exit.ExitCode()
		} else {
			t.Fatalf("cannot run helper process: %v", err)
		}
	}
	return helperResult{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func successes(results []helperResult) int {
	count := 0
	for _, result := range results {
		if result.code == 0 {
			count++
		}
	}
	return count
}

func successEnvelopeID(t *testing.T, payload string) string {
	t.Helper()
	var envelope struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil || envelope.Data.ID == "" {
		t.Fatalf("invalid success envelope: %v %s", err, payload)
	}
	return envelope.Data.ID
}

func errorEnvelopeCode(payload string) string {
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal([]byte(payload), &envelope)
	return envelope.Error.Code
}
