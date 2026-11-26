package tsec

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ghr07h/norns/norns/internal/extension/metadata"
)

const correctResult = `{"correct":true,"awarded":50,"cumulative_score":80,"correct_flag_count":2,"total_flag_count":3,"matched_flag_index":1,"future_field":"preserved"}`
const wrongResult = `{"correct":false,"awarded":0,"cumulative_score":80,"correct_flag_count":2,"total_flag_count":3,"matched_flag_index":null}`
const availableChallenge = `[{"unique_code":"challenge","container_status":"available"}]`

func TestSubmitAPIContract(t *testing.T) {
	for _, body := range []string{correctResult, wrongResult} {
		t.Run(body, func(t *testing.T) {
			calls := 0
			payload := SubmitRequest{UniqueCode: "web_sql_injection_01", Flag: " flag{中文<&\"\n>} "}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Header.Get("BENCHMARK_TOKEN") != "test-token" || r.Header.Get("Authorization") != "" {
					t.Error("incorrect authentication")
				}
				if calls == 1 {
					body, _ := io.ReadAll(r.Body)
					if r.Method != http.MethodGet || r.URL.Path != "/prefix/openapi/v1/challenges" || r.URL.RawQuery != "" || len(body) != 0 {
						t.Errorf("unexpected status check: %s %s body=%s", r.Method, r.URL, body)
					}
					io.WriteString(w, `[{"unique_code":"other","container_status":"stopped"},{"unique_code":"web_sql_injection_01","container_status":"available"}]`)
					return
				}
				if r.Method != http.MethodPost || r.URL.Path != "/prefix/openapi/v1/challenges/submit" || r.URL.RawQuery != "" {
					t.Errorf("unexpected route: %s %s", r.Method, r.URL)
				}
				if r.Header.Get("BENCHMARK_TOKEN") != "test-token" || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Authorization") != "" {
					t.Error("incorrect authentication or content type")
				}
				var got map[string]string
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil || len(got) != 2 || got["unique_code"] != payload.UniqueCode || got["flag"] != payload.Flag {
					t.Errorf("submission changed: %v err=%v", got, err)
				}
				io.WriteString(w, body)
			}))
			defer server.Close()
			client, err := New(Config{BaseURL: server.URL + "/prefix/", Token: "test-token"})
			if err != nil {
				t.Fatal(err)
			}
			result, err := client.Submit(context.Background(), payload)
			if err != nil || string(result) != body || calls != 2 {
				t.Fatalf("result=%s error=%v calls=%d", result, err, calls)
			}
		})
	}
}

func TestAPIErrorContractsAndNoRetry(t *testing.T) {
	for _, phase := range []string{"check", "submit"} {
		t.Run(phase, func(t *testing.T) {
			for _, test := range []struct {
				status int
				body   string
				code   string
			}{
				{404, `{"code":"task_not_found","message":"Unknown task","detail":{}}`, "task_not_found"},
				{404, `{"code":"challenge_not_found","message":"Unknown challenge","detail":{}}`, "challenge_not_found"},
				{409, `{"code":"invalid_state","message":"Task ended","detail":{}}`, "invalid_state"},
				{409, `{"code":"duplicate","message":"Flag repeated","detail":{"flag_index":1}}`, "duplicate"},
				{500, `{"code":"duplicate","message":"Unexpected error","detail":{}}`, "duplicate"},
				{503, `{"code":"resource_unavailable","message":"Try later","detail":{}}`, "resource_unavailable"},
				{500, `{"code":"internal_error","message":"Internal error","detail":{}}`, "internal_error"},
				{422, `{"detail":[{"loc":["body","flag"],"msg":"Rejected flag{secret} for test-token","type":"value_error","input":"flag{secret}"}]}`, "validation_error"},
				{429, `{"code":"rate_limited","message":"Slow down","detail":{}}`, "rate_limited"},
				{502, `<html>test-token flag{secret}</html>`, "http_error"},
			} {
				t.Run(test.code, func(t *testing.T) {
					calls := 0
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls++
						if phase == "submit" && r.Method == http.MethodGet {
							io.WriteString(w, availableChallenge)
							return
						}
						w.Header().Set("Retry-After", "2")
						w.WriteHeader(test.status)
						io.WriteString(w, test.body)
					}))
					defer server.Close()
					client, _ := New(Config{BaseURL: server.URL, Token: "test-token"})
					result, err := client.Submit(context.Background(), SubmitRequest{UniqueCode: "challenge", Flag: "flag{secret}"})
					wantCalls := 1
					if phase == "submit" {
						wantCalls = 2
					}
					if phase == "submit" && test.status == http.StatusConflict && test.code == "duplicate" {
						if err != nil || string(result) != `{"correct":true,"duplicate":true}` || calls != wantCalls {
							t.Fatalf("duplicate result=%s error=%v calls=%d", result, err, calls)
						}
						return
					}
					detail := assertCode(t, err, test.code)
					if detail.Status != test.status || detail.RetryAfter != "2" || calls != wantCalls {
						t.Fatalf("detail=%+v calls=%d", detail, calls)
					}
					encoded, _ := json.Marshal(detail)
					if strings.Contains(string(encoded), "test-token") || strings.Contains(string(encoded), "flag{secret}") {
						t.Fatal("error leaked credentials or submitted flag")
					}
					if test.status == 422 {
						if values, ok := detail.Detail.([]any); !ok || len(values) != 1 {
							t.Fatalf("FastAPI detail array lost: %v", detail.Detail)
						}
					}
				})
			}
		})
	}
}

func TestMalformedAndOversizedResults(t *testing.T) {
	for _, body := range []string{
		`null`, `{}`, `[]`, `<html>error</html>`, correctResult + `{}`,
		strings.Replace(correctResult, `"correct":true`, `"correct":null`, 1),
		strings.Replace(correctResult, `"awarded":50`, `"awarded":"50"`, 1),
		strings.Replace(correctResult, `"matched_flag_index":1,`, "", 1),
		strings.Replace(correctResult, `"matched_flag_index":1`, `"matched_flag_index":false`, 1),
		strings.Repeat("a", maxResponseBytes+1),
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				io.WriteString(w, availableChallenge)
				return
			}
			io.WriteString(w, body)
		}))
		client, _ := New(Config{BaseURL: server.URL, Token: "test-token"})
		_, err := client.Submit(context.Background(), SubmitRequest{UniqueCode: "challenge", Flag: "flag{secret}"})
		server.Close()
		assertCode(t, err, "invalid_response")
	}
}

func TestChallengeCheckBlocksSubmission(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		code string
	}{
		{"stopped", `[{"unique_code":"challenge","container_status":"stopped"}]`, "container_not_ready"},
		{"pending", `[{"unique_code":"challenge","container_status":"pending"}]`, "container_not_ready"},
		{"stop_pending", `[{"unique_code":"challenge","container_status":"stop_pending"}]`, "container_not_ready"},
		{"unknown_status", `[{"unique_code":"challenge","container_status":"unknown"}]`, "container_not_ready"},
		{"missing_status", `[{"unique_code":"challenge"}]`, "container_not_ready"},
		{"null_status", `[{"unique_code":"challenge","container_status":null}]`, "container_not_ready"},
		{"other_available", `[{"unique_code":"other","container_status":"available"},{"unique_code":"challenge","container_status":"stopped"}]`, "container_not_ready"},
		{"missing_challenge", `[{"unique_code":"other","container_status":"available"}]`, "challenge_not_found"},
		{"empty_list", `[]`, "challenge_not_found"},
		{"null_list", `null`, "invalid_response"},
		{"object", `{}`, "invalid_response"},
		{"invalid_json", `[`, "invalid_response"},
		{"wrong_status_type", `[{"unique_code":"challenge","container_status":true}]`, "invalid_response"},
		{"oversized", strings.Repeat("a", maxResponseBytes+1), "invalid_response"},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodGet || r.URL.Path != "/openapi/v1/challenges" || r.Header.Get("BENCHMARK_TOKEN") != "test-token" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
				}
				io.WriteString(w, test.body)
			}))
			defer server.Close()
			client, _ := New(Config{BaseURL: server.URL, Token: "test-token"})
			_, err := client.Submit(context.Background(), SubmitRequest{UniqueCode: "challenge", Flag: "secret"})
			detail := assertCode(t, err, test.code)
			if calls != 1 {
				t.Fatalf("expected only the preflight request, got %d calls", calls)
			}
			if test.name == "stopped" {
				if detail.Detail.(map[string]string)["container_status"] != "stopped" || !strings.Contains(detail.Message, "flag was not submitted") {
					t.Fatalf("missing container status: %+v", detail)
				}
			}
		})
	}
}

func TestTimeoutAndRedirectDoNotResubmitOrForwardCredentials(t *testing.T) {
	for _, phase := range []string{"check", "submit"} {
		t.Run(phase, func(t *testing.T) {
			var calls atomic.Int32
			slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if phase == "submit" && r.Method == http.MethodGet {
					io.WriteString(w, availableChallenge)
					return
				}
				io.Copy(io.Discard, r.Body)
				<-r.Context().Done()
			}))
			defer slow.Close()
			client, _ := New(Config{BaseURL: slow.URL, Token: "test-token"})
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()
			_, err := client.Submit(ctx, SubmitRequest{UniqueCode: "challenge", Flag: "secret"})
			detail := assertCode(t, err, "timeout")
			if phase == "check" && (calls.Load() != 1 || !strings.Contains(detail.Message, "flag was not submitted")) {
				t.Fatalf("preflight timeout sent a submission or misleading error: calls=%d error=%v", calls.Load(), detail)
			}
			if phase == "submit" && (calls.Load() != 2 || !strings.Contains(detail.Message, "platform may have received it")) {
				t.Fatalf("submission timeout lost receipt uncertainty: calls=%d error=%v", calls.Load(), detail)
			}
		})
	}

	forwarded := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded = true }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	client, _ := New(Config{BaseURL: redirect.URL, Token: "test-token"})
	_, err := client.Submit(context.Background(), SubmitRequest{UniqueCode: "challenge", Flag: "secret"})
	if detail := assertCode(t, err, "http_error"); detail.Status != 307 || forwarded {
		t.Fatalf("redirect followed: status=%d forwarded=%v", detail.Status, forwarded)
	}
}

func TestInputValidationAndEnvironment(t *testing.T) {
	client, _ := New(Config{BaseURL: "http://127.0.0.1:1", Token: "test-token"})
	for _, request := range []SubmitRequest{
		{Flag: "secret"}, {UniqueCode: " \t", Flag: "secret"},
		{UniqueCode: "challenge"}, {UniqueCode: "challenge", Flag: strings.Repeat("中", 4097)},
		{UniqueCode: "challenge", Flag: string([]byte{0xff})},
	} {
		_, err := client.Submit(context.Background(), request)
		assertCode(t, err, "invalid_input")
	}
	for _, flag := range []string{" ", strings.Repeat("中", 4096)} {
		if err := (SubmitRequest{UniqueCode: "challenge", Flag: flag}).Validate(); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []Config{
		{}, {BaseURL: "https://example.test"}, {Token: "test-token"},
		{BaseURL: "file:///tmp/platform", Token: "test-token"},
		{BaseURL: "https://user:password@example.test", Token: "test-token"},
		{BaseURL: "https://example.test?token=secret", Token: "test-token"},
		{BaseURL: "https://example.test#fragment", Token: "test-token"},
		{BaseURL: "https://example.test", Token: "first\r\nsecond"},
	} {
		_, err := New(test)
		assertCode(t, err, "config_error")
	}
	t.Setenv(BaseURLEnv, " https://example.test/prefix/ ")
	t.Setenv(TokenEnv, " test-token ")
	config, err := LoadConfigFromEnv()
	if err != nil || config.BaseURL != "https://example.test/prefix" || config.Token != "test-token" {
		t.Fatalf("unexpected config: %+v error=%v", config, err)
	}
}

func TestSourceMetadata(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := metadata.ParseSource(data, "tsec"); err != nil {
		t.Fatal(err)
	}
}

func assertCode(t *testing.T, err error, code string) *Error {
	t.Helper()
	var detail *Error
	if !errors.As(err, &detail) || detail.Code != code {
		t.Fatalf("expected %s, got %v", code, err)
	}
	return detail
}
