// Package tsec submits flags to the TSecBench API. It does not manage challenge
// containers or request hints.
package tsec

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

const RequestTimeout = 60 * time.Second
const maxResponseBytes = 1024 * 1024

type Error struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Status     int    `json:"status,omitempty"`
	Detail     any    `json:"detail,omitempty"`
	RetryAfter string `json:"retry_after,omitempty"`
}

func (e *Error) Error() string { return e.Message }

func failure(code, message string) *Error { return &Error{Code: code, Message: message} }

type SubmitRequest struct {
	UniqueCode string `json:"unique_code"`
	Flag       string `json:"flag"`
}

func (r SubmitRequest) Validate() error {
	if !utf8.ValidString(r.UniqueCode) || strings.TrimSpace(r.UniqueCode) == "" {
		return failure("invalid_input", "--unique-code must be a non-empty UTF-8 string")
	}
	if !utf8.ValidString(r.Flag) || utf8.RuneCountInString(r.Flag) < 1 || utf8.RuneCountInString(r.Flag) > 4096 {
		return failure("invalid_input", "--flag must contain 1 to 4096 Unicode characters")
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
		Timeout: RequestTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			// Custom authentication headers must not follow redirects.
			return http.ErrUseLastResponse
		},
	}}, nil
}

// Submit first checks that the challenge container is available, then returns
// the platform's submission JSON unchanged, including any future fields.
// A valid result with correct=false is a successful request. A submission's
// HTTP 409 duplicate response becomes {"correct":true,"duplicate":true}, since
// the flag was already accepted. This method never retries a submission.
func (c *Client) Submit(ctx context.Context, payload SubmitRequest) (json.RawMessage, error) {
	if err := payload.Validate(); err != nil {
		return nil, err
	}
	if err := c.checkChallenge(ctx, payload); err != nil {
		return nil, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, failure("invalid_input", "cannot encode flag submission")
	}
	body, err = c.request(ctx, http.MethodPost, "/openapi/v1/challenges/submit", body, payload.Flag)
	if err != nil {
		var apiError *Error
		if errors.As(err, &apiError) && apiError.Status == http.StatusConflict && apiError.Code == "duplicate" {
			// The duplicate response does not include scores or progress. Do not
			// invent those fields or imply that this request awarded new points.
			return json.RawMessage(`{"correct":true,"duplicate":true}`), nil
		}
		return nil, err
	}
	// Pointers distinguish a complete result from missing or null fields.
	var result struct {
		Correct          *bool           `json:"correct"`
		Awarded          *int            `json:"awarded"`
		CumulativeScore  *int            `json:"cumulative_score"`
		CorrectFlagCount *int            `json:"correct_flag_count"`
		TotalFlagCount   *int            `json:"total_flag_count"`
		MatchedFlagIndex json.RawMessage `json:"matched_flag_index"`
	}
	if err := json.Unmarshal(body, &result); err != nil || result.Correct == nil || result.Awarded == nil || result.CumulativeScore == nil || result.CorrectFlagCount == nil || result.TotalFlagCount == nil || len(result.MatchedFlagIndex) == 0 {
		return nil, failure("invalid_response", "TSecBench returned an invalid flag submission result")
	}
	var index *int
	if err := json.Unmarshal(result.MatchedFlagIndex, &index); err != nil {
		return nil, failure("invalid_response", "TSecBench returned an invalid matched_flag_index")
	}
	return json.RawMessage(body), nil
}

func (c *Client) checkChallenge(ctx context.Context, payload SubmitRequest) error {
	body, err := c.request(ctx, http.MethodGet, "/openapi/v1/challenges", nil, payload.Flag)
	if err != nil {
		return err
	}
	var challenges []struct {
		UniqueCode      string `json:"unique_code"`
		ContainerStatus string `json:"container_status"`
	}
	if err := json.Unmarshal(body, &challenges); err != nil || challenges == nil {
		return failure("invalid_response", "TSecBench returned an invalid challenge list; flag was not submitted")
	}
	for _, challenge := range challenges {
		if challenge.UniqueCode != payload.UniqueCode {
			continue
		}
		if challenge.ContainerStatus != "available" {
			return &Error{
				Code:    "container_not_ready",
				Message: "Challenge container is not available; flag was not submitted. Wait until the container is available before retrying",
				Detail:  map[string]string{"container_status": c.redactString(challenge.ContainerStatus, payload.Flag)},
			}
		}
		return nil
	}
	return failure("challenge_not_found", "Challenge was not found in the current task; flag was not submitted")
}

func (c *Client) request(ctx context.Context, method, path string, body []byte, flag string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, method, c.config.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, failure("request_error", "cannot create TSecBench request")
	}
	request.Header.Set("BENCHMARK_TOKEN", c.config.Token)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Accept", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return nil, requestError(ctx, err, method == http.MethodPost)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, requestError(ctx, err, method == http.MethodPost)
	}
	if len(data) > maxResponseBytes {
		return nil, failure("invalid_response", "TSecBench response exceeds the 1 MiB limit")
	}
	if response.StatusCode != http.StatusOK {
		return nil, c.apiError(response, data, flag)
	}
	return data, nil
}

func (c *Client) apiError(response *http.Response, body []byte, flag string) *Error {
	result := &Error{Code: "http_error", Message: fmt.Sprintf("TSecBench API returned HTTP %d", response.StatusCode), Status: response.StatusCode}
	if response.StatusCode == http.StatusUnprocessableEntity {
		result.Code, result.Message = "validation_error", "TSecBench rejected the request parameters"
	}
	var envelope struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Detail  any    `json:"detail"`
	}
	if json.Unmarshal(body, &envelope) == nil {
		if envelope.Code != "" {
			result.Code = envelope.Code
		}
		if envelope.Message != "" {
			result.Message = envelope.Message
		}
		result.Detail = c.redact(envelope.Detail, flag)
	}
	result.Code = c.redactString(result.Code, "")
	result.Message = c.redactString(result.Message, flag)
	result.RetryAfter = c.redactString(response.Header.Get("Retry-After"), "")
	return result
}

func (c *Client) redactString(value, flag string) string {
	value = strings.ReplaceAll(value, c.config.Token, "[REDACTED]")
	if flag != "" {
		value = strings.ReplaceAll(value, flag, "[REDACTED]")
	}
	return value
}

// FastAPI validation errors can include the submitted flag in detail[].input.
func (c *Client) redact(value any, flag string) any {
	switch value := value.(type) {
	case string:
		return c.redactString(value, flag)
	case []any:
		for i := range value {
			value[i] = c.redact(value[i], flag)
		}
	case map[string]any:
		for key, item := range value {
			value[key] = c.redact(item, flag)
		}
	}
	return value
}

func requestError(ctx context.Context, err error, submitting bool) error {
	operation, outcome := "challenge status check", "flag was not submitted"
	if submitting {
		operation, outcome = "flag submission", "the platform may have received it"
	}
	var networkError net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkError) && networkError.Timeout()) {
		return failure("timeout", "TSecBench "+operation+" timed out; "+outcome)
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return failure("request_error", "TSecBench "+operation+" was cancelled; "+outcome)
	}
	return failure("request_error", "TSecBench "+operation+" failed; "+outcome)
}
