package deepseeksearch

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSearchAPIContract(t *testing.T) {
	request := SearchRequest{Query: `模型 site:github.com OR site:huggingface.co lang:zh after:2025`, Count: 5, OriQuestion: "有哪些新模型？", Localization: "2001:db8::1", NeedContent: true}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/tools/search" || r.Header.Get("Authorization") != "Bearer test-secret" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected request: %s %s headers=%v", r.Method, r.URL.Path, r.Header)
		}
		var received SearchRequest
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil || received != request {
			t.Errorf("request=%+v err=%v", received, err)
		}
		io.WriteString(w, `{"results":[{"rerank_score":2.4,"title":"模型","url":"https://example.test/model","snippet":"short","summary":"long","content":"# Markdown","site_name":null,"site_icon":null,"date":"2026-06-01T00:00:00Z","authority":3}]}`)
	}))
	defer server.Close()
	client, err := New(Config{APIKey: "test-secret", BaseURL: server.URL + "/v1/"})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Search(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Results) != 1 || response.Results[0].Content != "# Markdown" || response.Results[0].RerankScore != 2.4 || response.Results[0].Authority != 3 || response.Results[0].SiteName != nil || response.Results[0].Date == nil {
		t.Fatalf("response lost fields: %+v", response)
	}
}

func TestFetchAPIContract(t *testing.T) {
	target := "https://example.test/article?q=中文"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var received map[string]string
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil || len(received) != 1 || received["url"] != target || r.URL.Path != "/v1/tools/fetch" {
			t.Errorf("path=%s body=%v err=%v", r.URL.Path, received, err)
		}
		io.WriteString(w, `{"title":"文章","content":"# Article\n\n正文"}`)
	}))
	defer server.Close()
	client, _ := New(Config{APIKey: "test-secret", BaseURL: server.URL})
	response, err := client.Fetch(context.Background(), target)
	if err != nil || response.Title != "文章" || response.Content != "# Article\n\n正文" {
		t.Fatalf("response=%+v err=%v", response, err)
	}
}

func TestSearchEmptyResultsAndInvalidResponses(t *testing.T) {
	for _, test := range []struct {
		body string
		ok   bool
	}{
		{`{"results":[]}`, true},
		{`{"results":null}`, false},
		{`{}`, false},
		{`null`, false},
		{`{"results":{}}`, false},
		{`<html>upstream error</html>`, false},
		{`{"results":[]} {}`, false},
	} {
		t.Run(test.body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, test.body) }))
			defer server.Close()
			client, _ := New(Config{APIKey: "test-secret", BaseURL: server.URL})
			response, err := client.Search(context.Background(), SearchRequest{Query: "test", Count: 1})
			if test.ok {
				if err != nil || response.Results == nil || len(response.Results) != 0 {
					t.Fatalf("response=%+v err=%v", response, err)
				}
			} else {
				assertErrorCode(t, err, "invalid_response")
			}
		})
	}
}

func TestFetchRejectsMissingFields(t *testing.T) {
	for _, body := range []string{`{}`, `null`, `{"title":"a"}`, `{"title":"a","content":null}`, `{"title":false,"content":"a"}`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body) }))
			defer server.Close()
			client, _ := New(Config{APIKey: "test-secret", BaseURL: server.URL})
			_, err := client.Fetch(context.Background(), "https://example.test")
			assertErrorCode(t, err, "invalid_response")
		})
	}
}

func TestAPIErrorsKeepStatusWithoutRetryingOrLeakingKey(t *testing.T) {
	for _, status := range []int{400, 401, 402, 403, 404, 408, 413, 429, 500, 504} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Retry-After", "2")
				w.WriteHeader(status)
				io.WriteString(w, `{"error":{"message":"problem with test-secret","type":"upstream_error","param":null,"code":"rate_limit"}}`)
			}))
			defer server.Close()
			client, _ := New(Config{APIKey: "test-secret", BaseURL: server.URL})
			_, err := client.Search(context.Background(), SearchRequest{Query: "test", Count: 1})
			var detail *Error
			if !errors.As(err, &detail) || detail.Code != "api_error" || detail.Status != status || detail.RetryAfter != "2" || detail.APICode != "rate_limit" || detail.Type != "upstream_error" || calls != 1 || strings.Contains(detail.Message, "test-secret") {
				t.Fatalf("err=%+v calls=%d", err, calls)
			}
		})
	}
}

func TestRequestDeadlineAndRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	}))
	defer server.Close()
	client, _ := New(Config{APIKey: "test-secret", BaseURL: server.URL})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := client.Search(ctx, SearchRequest{Query: "test", Count: 1})
	assertErrorCode(t, err, "timeout")
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, server.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	client, _ = New(Config{APIKey: "test-secret", BaseURL: redirect.URL})
	_, err = client.Search(context.Background(), SearchRequest{Query: "test", Count: 1})
	assertErrorCode(t, err, "api_error")
}

func TestInputValidationBeforeNetwork(t *testing.T) {
	client, _ := New(Config{APIKey: "test-secret", BaseURL: "http://127.0.0.1:1"})
	for _, request := range []SearchRequest{
		{Query: " ", Count: 1}, {Query: strings.Repeat("中", 129), Count: 1},
		{Query: "a", Count: 0}, {Query: "a", Count: 51},
		{Query: "a", Count: 1, OriQuestion: strings.Repeat("文", 129)},
		{Query: "a", Count: 1, Localization: "not-an-ip"},
	} {
		_, err := client.Search(context.Background(), request)
		assertErrorCode(t, err, "invalid_input")
	}
	if err := (SearchRequest{Query: strings.Repeat("中", 128), Count: 50}).Validate(); err != nil {
		t.Fatalf("Unicode character count should not use byte count: %v", err)
	}
	for _, target := range []string{"", "file:///etc/passwd", "https:///missing-host", "https://user:password@example.test", "https://example.test/" + strings.Repeat("a", 8192)} {
		_, err := client.Fetch(context.Background(), target)
		assertErrorCode(t, err, "invalid_input")
	}
}

func assertErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	var detail *Error
	if !errors.As(err, &detail) || detail.Code != code {
		t.Fatalf("expected %s, got %v", code, err)
	}
}
