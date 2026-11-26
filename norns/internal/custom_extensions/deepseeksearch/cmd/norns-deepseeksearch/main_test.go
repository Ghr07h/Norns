package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ghr07h/norns/norns/internal/custom_extensions/deepseeksearch"
)

type fakeClient struct {
	request deepseeksearch.SearchRequest
	target  string
	err     error
}

func (f *fakeClient) Search(_ context.Context, request deepseeksearch.SearchRequest) (*deepseeksearch.SearchResponse, error) {
	f.request = request
	return &deepseeksearch.SearchResponse{Results: []deepseeksearch.Result{{Title: "Norns", Summary: "Details"}}}, f.err
}

func (f *fakeClient) Fetch(_ context.Context, target string) (*deepseeksearch.FetchResponse, error) {
	f.target = target
	return &deepseeksearch.FetchResponse{Title: "Norns", Content: "# Norns\n\nContent"}, f.err
}

func TestSearchAndFetchCommands(t *testing.T) {
	for _, args := range [][]string{
		{"--query", "模型 site:github.com", "--need-content", "--count", "5", "--ori-question", "原问题", "--localization", "127.0.0.1"},
		{"search", "--query", "Norns", "-m", "5"},
		{"--query", "Norns", "--max-results", "5"},
		{"fetch", "--url", "https://example.test"},
	} {
		fake := &fakeClient{}
		var stdout, stderr bytes.Buffer
		code := run(args, &stdout, &stderr, func() (searchClient, error) { return fake, nil })
		if code != 0 || stderr.Len() != 0 || !json.Valid(stdout.Bytes()) {
			t.Fatalf("args=%v code=%d stdout=%q stderr=%q", args, code, stdout.String(), stderr.String())
		}
		if args[0] == "fetch" {
			if fake.target != "https://example.test" || !strings.Contains(stdout.String(), `"content":"# Norns\n\nContent"`) {
				t.Fatalf("target=%s stdout=%s", fake.target, stdout.String())
			}
		} else if fake.request.Count != 5 || fake.request.Query == "" || !strings.Contains(stdout.String(), `"results":[`) {
			t.Fatalf("request=%+v stdout=%s", fake.request, stdout.String())
		}
		if args[0] == "--query" && args[1] == "模型 site:github.com" && (!fake.request.NeedContent || fake.request.OriQuestion != "原问题" || fake.request.Localization != "127.0.0.1") {
			t.Fatalf("optional fields lost: %+v", fake.request)
		}
	}
}

func TestHelpAndInvalidInputDoNotRequireConfiguration(t *testing.T) {
	t.Setenv(deepseeksearch.ConfigEnv, "")
	for _, args := range [][]string{{"--help"}, {"search", "--help"}, {"fetch", "--help"}} {
		var stdout, stderr bytes.Buffer
		code := run(args, &stdout, &stderr, func() (searchClient, error) { t.Fatal("help must not read configuration"); return nil, nil })
		if code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "NORNS_EXTENSION_DEEPSEEKSEARCH_CONFIG") {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
		}
	}
	for _, args := range [][]string{
		{}, {"--query", ""}, {"--query", "Norns", "--count", "51"},
		{"--query", "Norns", "--count", "bad"}, {"--query", "Norns", "--ori-question", ""},
		{"--query", "Norns", "--unknown"}, {"--query", "Norns", "extra"},
		{"fetch"}, {"fetch", "--url", "file:///tmp/a"}, {"fetch", "--query", "bad"},
	} {
		var stdout, stderr bytes.Buffer
		code := run(args, &stdout, &stderr, func() (searchClient, error) { t.Fatal("invalid input must not read configuration"); return nil, nil })
		if code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), `"code":"invalid_input"`) {
			t.Fatalf("args=%v code=%d stdout=%q stderr=%q", args, code, stdout.String(), stderr.String())
		}
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--query", "Norns"}, &stdout, &stderr, configuredClient); code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), `"code":"config_error"`) {
		t.Fatalf("missing configuration: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestConfiguredCommandAgainstLocalAPI(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer local-test-key" {
			t.Error("missing Bearer authentication")
		}
		if r.URL.Path == "/v1/tools/fetch" {
			io.WriteString(w, `{"title":"Article","content":"# Article\n\nText"}`)
		} else {
			io.WriteString(w, `{"results":[]}`)
		}
	}))
	defer server.Close()
	filename := filepath.Join(t.TempDir(), "deepseeksearch.yaml")
	if err := os.WriteFile(filename, []byte("api_key: local-test-key\nbase_url: "+server.URL+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(deepseeksearch.ConfigEnv, filename)
	for _, args := range [][]string{{"--query", "Norns"}, {"fetch", "--url", "https://example.test"}} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr, configuredClient); code != 0 || stderr.Len() != 0 || !json.Valid(stdout.Bytes()) {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
		}
	}
	if calls != 2 {
		t.Fatalf("expected exactly two API calls, got %d", calls)
	}
}

func TestCommandReportsRateLimit(t *testing.T) {
	fake := &fakeClient{err: &deepseeksearch.Error{Code: "api_error", Message: "Rate limited", Status: 429, RetryAfter: "2"}}
	var stdout, stderr bytes.Buffer
	code := run([]string{"--query", "Norns"}, &stdout, &stderr, func() (searchClient, error) { return fake, nil })
	if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), `"status":429`) || !strings.Contains(stderr.String(), `"retry_after":"2"`) {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
