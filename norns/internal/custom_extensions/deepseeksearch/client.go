package deepseeksearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

const DefaultCount = 10
const maxResponseBytes = 32 * 1024 * 1024

type Error struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Status     int    `json:"status,omitempty"`
	Type       string `json:"type,omitempty"`
	APICode    string `json:"api_code,omitempty"`
	RetryAfter string `json:"retry_after,omitempty"`
}

func (e *Error) Error() string { return e.Message }

func failure(code, message string) *Error { return &Error{Code: code, Message: message} }

type SearchRequest struct {
	Query        string `json:"query"`
	Count        int    `json:"count"`
	OriQuestion  string `json:"ori_question,omitempty"`
	Localization string `json:"localization,omitempty"`
	NeedContent  bool   `json:"need_content"`
}

func (r SearchRequest) Validate() error {
	if !utf8.ValidString(r.Query) || strings.TrimSpace(r.Query) == "" || utf8.RuneCountInString(r.Query) > 128 {
		return failure("invalid_input", "query must contain 1 to 128 Unicode characters and must not be blank")
	}
	if r.Count < 1 || r.Count > 50 {
		return failure("invalid_input", "count must be between 1 and 50")
	}
	if !utf8.ValidString(r.OriQuestion) || utf8.RuneCountInString(r.OriQuestion) > 128 {
		return failure("invalid_input", "ori-question must contain 1 to 128 Unicode characters when provided")
	}
	if r.Localization != "" && net.ParseIP(r.Localization) == nil {
		return failure("invalid_input", "localization must be an IP address")
	}
	return nil
}

type Result struct {
	RerankScore float64 `json:"rerank_score"`
	Title       string  `json:"title"`
	URL         string  `json:"url"`
	Snippet     string  `json:"snippet"`
	Summary     string  `json:"summary"`
	Content     string  `json:"content"`
	SiteName    *string `json:"site_name"`
	SiteIcon    *string `json:"site_icon"`
	Date        *string `json:"date"`
	Authority   int     `json:"authority"`
}

type SearchResponse struct {
	Results []Result `json:"results"`
}

type FetchResponse struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

func ValidateFetchURL(value string) error {
	parsed, err := url.Parse(value)
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > 8192 || err != nil || !validHTTPURL(parsed) {
		return failure("invalid_input", "url must be an HTTP(S) URL of at most 8192 characters with a host and no credentials")
	}
	return nil
}

type Client struct {
	config Config
	http   *http.Client
}

func New(config Config) (*Client, error) {
	config, err := normalizeConfig(config)
	if err != nil {
		return nil, err
	}
	return &Client{config: config, http: &http.Client{
		Timeout: time.Duration(config.TimeoutSeconds) * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}, nil
}

func (c *Client) Search(ctx context.Context, request SearchRequest) (*SearchResponse, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var response SearchResponse
	if err := c.post(ctx, "search", request, &response); err != nil {
		return nil, err
	}
	if response.Results == nil {
		return nil, failure("invalid_response", "DeepSeek Search response is missing a results array")
	}
	return &response, nil
}

func (c *Client) Fetch(ctx context.Context, target string) (*FetchResponse, error) {
	if err := ValidateFetchURL(target); err != nil {
		return nil, err
	}
	// Pointers distinguish an invalid response from valid empty strings.
	var response struct {
		Title   *string `json:"title"`
		Content *string `json:"content"`
	}
	if err := c.post(ctx, "fetch", struct {
		URL string `json:"url"`
	}{target}, &response); err != nil {
		return nil, err
	}
	if response.Title == nil || response.Content == nil {
		return nil, failure("invalid_response", "DeepSeek Fetch response is missing title or content")
	}
	return &FetchResponse{Title: *response.Title, Content: *response.Content}, nil
}

func (c *Client) post(ctx context.Context, tool string, payload, result any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return failure("invalid_input", "cannot encode request")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.BaseURL+"/v1/tools/"+tool, bytes.NewReader(body))
	if err != nil {
		return failure("request_error", "cannot create DeepSeek request")
	}
	request.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return requestError(ctx, err)
	}
	defer response.Body.Close()
	body, err = io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return requestError(ctx, err)
	}
	if len(body) > maxResponseBytes {
		return failure("invalid_response", "DeepSeek response exceeds the 32 MiB limit")
	}
	if response.StatusCode != http.StatusOK {
		apiError := &Error{Code: "api_error", Status: response.StatusCode, Message: fmt.Sprintf("DeepSeek API returned HTTP %d", response.StatusCode), RetryAfter: response.Header.Get("Retry-After")}
		var envelope struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    string `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal(body, &envelope) == nil {
			if envelope.Error.Message != "" {
				apiError.Message += ": " + envelope.Error.Message
			}
			apiError.Type, apiError.APICode = envelope.Error.Type, envelope.Error.Code
		}
		apiError.Message = c.redact(apiError.Message)
		apiError.Type, apiError.APICode, apiError.RetryAfter = c.redact(apiError.Type), c.redact(apiError.APICode), c.redact(apiError.RetryAfter)
		return apiError
	}
	if err := json.Unmarshal(body, result); err != nil {
		return failure("invalid_response", "DeepSeek API returned an invalid JSON response")
	}
	return nil
}

func (c *Client) redact(value string) string {
	return strings.ReplaceAll(value, c.config.APIKey, "[REDACTED]")
}

func requestError(ctx context.Context, err error) error {
	var networkError net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkError) && networkError.Timeout()) {
		return failure("timeout", "DeepSeek request timed out")
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return failure("request_error", "DeepSeek request was cancelled")
	}
	return failure("request_error", "DeepSeek request failed")
}
