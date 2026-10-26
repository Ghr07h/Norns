package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Ghr07h/norns/norns/internal/websearch"
)

type fakeClient struct {
	query   string
	maximum int
	results []websearch.Result
	err     error
}

func (f *fakeClient) Search(_ context.Context, query string, maximum int) ([]websearch.Result, error) {
	f.query, f.maximum = query, maximum
	return f.results, f.err
}

func TestRunOutputsMVPCompatibleJSON(t *testing.T) {
	fake := &fakeClient{results: []websearch.Result{{Title: "Norns", Href: "https://example.test", Body: "Result"}}}
	var proxy string
	var stdout, stderr bytes.Buffer
	code := run([]string{"--query", "  Norns graph  ", "-m", "3", "--proxy", "socks5://localhost:1080"}, &stdout, &stderr, func(value string) (searchClient, error) {
		proxy = value
		return fake, nil
	})
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if proxy != "socks5://localhost:1080" || fake.query != "Norns graph" || fake.maximum != 3 {
		t.Fatalf("proxy=%q query=%q maximum=%d", proxy, fake.query, fake.maximum)
	}
	var results []websearch.Result
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil || len(results) != 1 {
		t.Fatalf("invalid output %q: %v", stdout.String(), err)
	}
}

func TestRunReturnsStructuredInvalidInput(t *testing.T) {
	for _, args := range [][]string{
		{"--query", ""},
		{"--query", "Norns", "-m", "0"},
		{"--query", "Norns", "-m", "not-a-number"},
		{"--query", "Norns", "extra"},
	} {
		var stdout, stderr bytes.Buffer
		code := run(args, &stdout, &stderr, func(string) (searchClient, error) {
			t.Fatal("factory must not run for invalid input")
			return nil, nil
		})
		if code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), `"code":"invalid_input"`) {
			t.Fatalf("args=%v code=%d stdout=%q stderr=%q", args, code, stdout.String(), stderr.String())
		}
	}
}

func TestRunMapsSearchErrors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := &fakeClient{err: errors.New("backend unavailable")}
	code := run([]string{"--query", "Norns"}, &stdout, &stderr, func(string) (searchClient, error) { return fake, nil })
	if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), `"code":"search_error"`) || !strings.Contains(stderr.String(), "backend unavailable") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunHelpDoesNotConstructClient(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--help"}, &stdout, &stderr, func(string) (searchClient, error) {
		return nil, errors.New("must not run")
	})
	if code != 0 || stderr.Len() != 0 || !strings.HasPrefix(stdout.String(), "Usage: norns websearch") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
