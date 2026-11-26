package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ghr07h/norns/norns/internal/custom_extensions/tsec"
)

func TestHelpAndInvalidInputDoNotReadConfiguration(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}} {
		var stdout, stderr bytes.Buffer
		code := run(args, &stdout, &stderr, func() (submitClient, error) { t.Fatal("help read configuration"); return nil, nil })
		if code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "BENCHMARK_TOKEN") {
			t.Fatalf("help code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
		}
	}
	for _, args := range [][]string{
		{}, {"--unique-code", "challenge"}, {"--flag", "secret"},
		{"--unique-code", "challenge", "--flag", ""},
		{"--unique-code", "challenge", "--flag", strings.Repeat("中", 4097)},
		{"--unique-code", "challenge", "--flag", "secret", "extra"},
		{"--unique-code", "challenge", "--flag", "secret", "--unknown"},
	} {
		var stdout, stderr bytes.Buffer
		code := run(args, &stdout, &stderr, func() (submitClient, error) { t.Fatal("invalid input read configuration"); return nil, nil })
		if code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), `"code":"invalid_input"`) {
			t.Fatalf("invalid input code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
		}
	}
}

func TestConfiguredCommandAndAPIErrors(t *testing.T) {
	for _, test := range []struct {
		status int
		body   string
		code   int
		stdout string
	}{
		{200, `{"correct":true,"awarded":50,"cumulative_score":80,"correct_flag_count":2,"total_flag_count":3,"matched_flag_index":1}`, 0, ""},
		{200, `{"correct":false,"awarded":0,"cumulative_score":80,"correct_flag_count":2,"total_flag_count":3,"matched_flag_index":null}`, 0, ""},
		{409, `{"code":"duplicate","message":"Already submitted","detail":{}}`, 0, `{"correct":true,"duplicate":true}`},
		{409, `{"code":"invalid_state","message":"Task ended","detail":{}}`, 1, ""},
		{422, `{"detail":[{"loc":["body","flag"],"msg":"Rejected","type":"value_error"}]}`, 1, ""},
	} {
		t.Run(test.body, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method == http.MethodGet && r.URL.Path == "/openapi/v1/challenges" {
					io.WriteString(w, `[{"unique_code":"challenge","container_status":"available"}]`)
					return
				}
				if r.Method != "POST" || r.URL.Path != "/openapi/v1/challenges/submit" || r.Header.Get("BENCHMARK_TOKEN") != "test-token" {
					t.Error("incorrect submission route or authentication")
				}
				var payload tsec.SubmitRequest
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.UniqueCode != "challenge" || payload.Flag != " flag{a&b} " {
					t.Errorf("payload=%+v error=%v", payload, err)
				}
				w.WriteHeader(test.status)
				io.WriteString(w, test.body)
			}))
			defer server.Close()
			t.Setenv(tsec.BaseURLEnv, server.URL)
			t.Setenv(tsec.TokenEnv, "test-token")
			var stdout, stderr bytes.Buffer
			code := run([]string{"--unique-code", "challenge", "--flag", " flag{a&b} "}, &stdout, &stderr, configuredClient)
			if code != test.code || calls != 2 {
				t.Fatalf("exit=%d calls=%d stdout=%s stderr=%s", code, calls, &stdout, &stderr)
			}
			if test.code == 0 {
				want := test.body
				if test.stdout != "" {
					want = test.stdout
				}
				if strings.TrimSpace(stdout.String()) != want || stderr.Len() != 0 {
					t.Fatalf("unexpected success output: stdout=%s stderr=%s", &stdout, &stderr)
				}
			} else if stdout.Len() != 0 || !json.Valid(stderr.Bytes()) || !strings.Contains(stderr.String(), `"status":`) {
				t.Fatalf("unexpected error output: stdout=%s stderr=%s", &stdout, &stderr)
			}
		})
	}
}

func TestUnavailableChallengeReturnsErrorWithoutSubmitting(t *testing.T) {
	for _, status := range []string{"pending", "stop_pending", "stopped"} {
		t.Run(status, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodGet || r.URL.Path != "/openapi/v1/challenges" || r.Header.Get("BENCHMARK_TOKEN") != "test-token" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
				}
				json.NewEncoder(w).Encode([]map[string]string{{"unique_code": "challenge", "container_status": status}})
			}))
			defer server.Close()
			t.Setenv(tsec.BaseURLEnv, server.URL)
			t.Setenv(tsec.TokenEnv, "test-token")
			var stdout, stderr bytes.Buffer
			code := run([]string{"--unique-code", "challenge", "--flag", "secret"}, &stdout, &stderr, configuredClient)
			var envelope struct {
				Error tsec.Error `json:"error"`
			}
			if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if code != 1 || calls != 1 || stdout.Len() != 0 || envelope.Error.Code != "container_not_ready" {
				t.Fatalf("exit=%d calls=%d stdout=%s stderr=%s", code, calls, &stdout, &stderr)
			}
			if envelope.Error.Detail.(map[string]any)["container_status"] != status {
				t.Fatalf("missing container status: %s", &stderr)
			}
		})
	}
}

type fixedClient struct{}

func (fixedClient) Submit(context.Context, tsec.SubmitRequest) (json.RawMessage, error) {
	return json.RawMessage(`{"correct":true}`), nil
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("broken output") }

func TestConfigurationAndOutputErrors(t *testing.T) {
	t.Setenv(tsec.BaseURLEnv, "")
	t.Setenv(tsec.TokenEnv, "")
	args := []string{"--unique-code", "challenge", "--flag", "secret"}
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr, configuredClient); code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), `"code":"config_error"`) {
		t.Fatalf("config exit=%d stdout=%s stderr=%s", code, &stdout, &stderr)
	}
	stderr.Reset()
	if code := run(args, brokenWriter{}, &stderr, func() (submitClient, error) { return fixedClient{}, nil }); code != 1 || !strings.Contains(stderr.String(), `"code":"output_error"`) {
		t.Fatalf("output exit=%d stderr=%s", code, &stderr)
	}
}
