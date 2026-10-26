package websearch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestNewSelectsBackendFromExplicitProxy(t *testing.T) {
	direct, err := New("")
	if err != nil {
		t.Fatal(err)
	}
	if direct.BackendName() != "baidu" {
		t.Fatalf("direct backend = %q", direct.BackendName())
	}
	proxied, err := New("socks5h://127.0.0.1:1080")
	if err != nil {
		t.Fatal(err)
	}
	if proxied.BackendName() != "google" {
		t.Fatalf("proxied backend = %q", proxied.BackendName())
	}
}

func TestProxyValidation(t *testing.T) {
	for _, value := range []string{
		"http://127.0.0.1:7890",
		"socks5://localhost:1080",
		"socks5h://user:password@localhost:1080",
	} {
		if _, err := validateProxy(value); err != nil {
			t.Fatalf("valid proxy %q rejected: %v", value, err)
		}
	}
	for _, value := range []string{
		"https://127.0.0.1:7890",
		"socks://127.0.0.1:1080",
		"127.0.0.1:7890",
		"http://127.0.0.1",
		"http://127.0.0.1:invalid",
		"http://127.0.0.1:7890/path",
		"http://127.0.0.1:70000",
	} {
		if _, err := validateProxy(value); ErrorCode(err) != "invalid_input" {
			t.Fatalf("invalid proxy %q returned %v", value, err)
		}
	}
}

func TestParseBaiduResults(t *testing.T) {
	results, err := parseBaiduResults([]byte(`
<html><body>
  <div class="result c-container" mu="https://example.test/direct">
    <h3 class="t"><a href="https://example.test/norns"><em>Norns</em> 项目</a></h3>
    <div data-module="abstract">用于测试 <em>Baidu</em> 后端。</div>
  </div>
  <div class="result c-container">
    <h3><a href="http://www.baidu.com/link?url=opaque">中转结果</a></h3>
    <div class="c-abstract">没有真实地址时不应返回。</div>
  </div>
</body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0] != (Result{Title: "Norns 项目", Href: "https://example.test/direct", Body: "用于测试 Baidu 后端。"}) {
		t.Fatalf("unexpected results: %#v", results)
	}
}

func TestBaiduSecurityVerification(t *testing.T) {
	_, err := parseBaiduResults([]byte("<title>百度安全验证</title>"))
	if err == nil || !strings.Contains(err.Error(), "security verification") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBaiduBackendPaginatesAndUsesExpectedParameters(t *testing.T) {
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.URL.Query().Get("wd") != "Norns graph" || r.URL.Query().Get("ie") != "utf-8" || r.URL.Query().Get("rn") != "10" {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		start := r.URL.Query().Get("pn")
		if requests == 1 && start != "0" || requests == 2 && start != "10" {
			t.Errorf("unexpected page offset %q", start)
		}
		var body strings.Builder
		for index := 0; index < 10; index++ {
			absolute := (requests-1)*10 + index
			fmt.Fprintf(&body, `<div class="c-container" mu="https://example.test/%d"><h3><a>Result %d</a></h3><div class="c-abstract">Norns graph body</div></div>`, absolute, absolute)
		}
		return htmlResponse(body.String()), nil
	})}
	backend := newBaiduBackend()
	backend.endpoint = "https://baidu.test/search"
	searcher := NewSearcher(client, backend)
	results, err := searcher.Search(context.Background(), "Norns graph", 12)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || len(results) != 12 {
		t.Fatalf("requests=%d results=%d", requests, len(results))
	}
}

func TestParseGoogleWMLResults(t *testing.T) {
	results, err := parseGoogleResults([]byte(`
<?xml version="1.0"?>
<html><body>
  <div>
    <div><a href="/url?q=https%3A%2F%2Fexample.test%2Fnorns&amp;sa=U"><span>Norns result</span></a></div>
    <div><table><tr><td>Google result <b>body</b></td></tr></table></div>
  </div>
  <div>
    <div><a href="/search?q=norns"><span>Internal</span></a></div>
    <div><table><tr><td>Ignored</td></tr></table></div>
  </div>
</body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0] != (Result{Title: "Norns result", Href: "https://example.test/norns", Body: "Google result body"}) {
		t.Fatalf("unexpected results: %#v", results)
	}
}

func TestGoogleBackendUsesDDGSParametersAndHeaders(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		query := r.URL.Query()
		want := url.Values{"q": {"Norns"}, "sca_esv": {"1"}, "filter": {"1"}, "start": {"0"}, "hl": {"en-US"}, "lr": {"lang_en"}, "cr": {"countryUS"}}
		if query.Encode() != want.Encode() {
			t.Errorf("query = %q, want %q", query.Encode(), want.Encode())
		}
		if !strings.Contains(r.UserAgent(), "NokiaN72") || r.Header.Get("Cookie") != "CONSENT=YES+" {
			t.Errorf("unexpected headers: %#v", r.Header)
		}
		return htmlResponse(`<div><div><a href="https://example.test"><span>Norns</span></a></div><div><table><tr><td>Body</td></tr></table></div></div>`), nil
	})}
	backend := newGoogleBackend()
	backend.endpoint = "https://google.test/search"
	searcher := NewSearcher(client, backend)
	results, err := searcher.Search(context.Background(), "Norns", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Href != "https://example.test" {
		t.Fatalf("unexpected results: %#v", results)
	}
}

func TestNormalizeDeduplicateAndRank(t *testing.T) {
	results := normalizeAndDeduplicate([]Result{
		{Title: "  First\nresult ", Href: "https://example.test/a", Body: "short"},
		{Title: "First result", Href: "https://example.test/a", Body: "a longer Norns body"},
		{Title: "Norns wiki", Href: "https://en.wikipedia.org/wiki/Norns", Body: "reference"},
		{Title: "", Href: "https://example.test/invalid", Body: "ignored"},
	})
	results = rank(results, "Norns")
	if len(results) != 2 || !strings.Contains(results[0].Href, "wikipedia.org") || results[1].Body != "a longer Norns body" {
		t.Fatalf("unexpected normalized results: %#v", results)
	}
}

func TestSearchRejectsInvalidInputAndEmptyResults(t *testing.T) {
	searcher := NewSearcher(http.DefaultClient, staticBackend{})
	if _, err := searcher.Search(context.Background(), " ", 10); ErrorCode(err) != "invalid_input" {
		t.Fatalf("empty query returned %v", err)
	}
	if _, err := searcher.Search(context.Background(), "Norns", 0); ErrorCode(err) != "invalid_input" {
		t.Fatalf("invalid maximum returned %v", err)
	}
	if _, err := searcher.Search(context.Background(), "Norns", 10); ErrorCode(err) != "search_error" {
		t.Fatalf("empty result returned %v", err)
	}
}

func TestPageCountDoesNotOverflow(t *testing.T) {
	maximum := int(^uint(0) >> 1)
	if pages := pageCount(maximum); pages < 1 {
		t.Fatalf("page count overflowed: %d", pages)
	}
}

type staticBackend struct{}

func (staticBackend) Name() string { return "static" }
func (staticBackend) Search(context.Context, *http.Client, string, int) ([]Result, error) {
	return nil, nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func htmlResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
