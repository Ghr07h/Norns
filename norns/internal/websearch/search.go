package websearch

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode"
)

const DefaultMaxResults = 10

type Result struct {
	Title string `json:"title"`
	Href  string `json:"href"`
	Body  string `json:"body"`
}

type Error struct {
	Code    string
	Message string
	Cause   error
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.Cause }

func errorf(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

func wrap(code, message string, cause error) *Error {
	return &Error{Code: code, Message: message, Cause: cause}
}

func ErrorCode(err error) string {
	var searchErr *Error
	if errors.As(err, &searchErr) {
		return searchErr.Code
	}
	return "search_error"
}

type Backend interface {
	Name() string
	Search(context.Context, *http.Client, string, int) ([]Result, error)
}

type Searcher struct {
	client  *http.Client
	backend Backend
}

func New(proxyURL string) (*Searcher, error) {
	client, err := newHTTPClient(proxyURL)
	if err != nil {
		return nil, err
	}
	var backend Backend = newBaiduBackend()
	if proxyURL != "" {
		backend = newGoogleBackend()
	}
	return &Searcher{client: client, backend: backend}, nil
}

func NewSearcher(client *http.Client, backend Backend) *Searcher {
	return &Searcher{client: client, backend: backend}
}

func (s *Searcher) BackendName() string { return s.backend.Name() }

func (s *Searcher) Search(ctx context.Context, query string, maxResults int) ([]Result, error) {
	query = normalizeText(query)
	if query == "" {
		return nil, errorf("invalid_input", "query must not be empty")
	}
	if maxResults < 1 {
		return nil, errorf("invalid_input", "max results must be a positive integer")
	}
	results, err := s.backend.Search(ctx, s.client, query, maxResults)
	if err != nil {
		var searchErr *Error
		if errors.As(err, &searchErr) {
			return nil, searchErr
		}
		return nil, wrap("search_error", err.Error(), err)
	}
	results = normalizeAndDeduplicate(results)
	results = rank(results, query)
	if len(results) == 0 {
		return nil, errorf("search_error", "no results found")
	}
	if len(results) > maxResults {
		results = results[:maxResults]
	}
	return results, nil
}

func normalizeAndDeduplicate(results []Result) []Result {
	out := make([]Result, 0, len(results))
	positions := make(map[string]int, len(results))
	for _, result := range results {
		result.Title = normalizeText(result.Title)
		result.Href = normalizeURL(result.Href)
		result.Body = normalizeText(result.Body)
		if result.Title == "" || result.Href == "" {
			continue
		}
		if parsed, err := url.Parse(result.Href); err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
			continue
		}
		if index, exists := positions[result.Href]; exists {
			if len(result.Body) > len(out[index].Body) {
				out[index] = result
			}
			continue
		}
		positions[result.Href] = len(out)
		out = append(out, result)
	}
	return out
}

func normalizeText(value string) string {
	value = strings.ToValidUTF8(value, "")
	value = strings.Map(func(r rune) rune {
		if unicode.In(r, unicode.Cc, unicode.Cf, unicode.Co, unicode.Cs) && !unicode.IsSpace(r) {
			return -1
		}
		return r
	}, value)
	return strings.Join(strings.Fields(value), " ")
}

func normalizeURL(value string) string {
	value = strings.TrimSpace(value)
	if decoded, err := url.PathUnescape(value); err == nil {
		value = decoded
	}
	return strings.ReplaceAll(value, " ", "+")
}

func rank(results []Result, query string) []Result {
	tokens := queryTokens(query)
	buckets := make([][]Result, 5)
	for _, result := range results {
		if strings.Contains(result.Title, "Category:") && strings.Contains(result.Title, "Wikimedia") {
			continue
		}
		if strings.Contains(strings.ToLower(result.Href), "wikipedia.org") {
			buckets[0] = append(buckets[0], result)
			continue
		}
		titleHit := containsToken(result.Title, tokens)
		bodyHit := containsToken(result.Body, tokens)
		switch {
		case titleHit && bodyHit:
			buckets[1] = append(buckets[1], result)
		case titleHit:
			buckets[2] = append(buckets[2], result)
		case bodyHit:
			buckets[3] = append(buckets[3], result)
		default:
			buckets[4] = append(buckets[4], result)
		}
	}
	out := make([]Result, 0, len(results))
	for _, bucket := range buckets {
		out = append(out, bucket...)
	}
	return out
}

func queryTokens(query string) []string {
	parts := strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return r != '_' && !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	tokens := make([]string, 0, len(parts))
	for _, part := range parts {
		if len([]rune(part)) >= 3 {
			tokens = append(tokens, part)
		}
	}
	return tokens
}

func containsToken(value string, tokens []string) bool {
	value = strings.ToLower(value)
	for _, token := range tokens {
		if strings.Contains(value, token) {
			return true
		}
	}
	return false
}
