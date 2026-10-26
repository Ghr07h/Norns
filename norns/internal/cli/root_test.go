package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestCLIWorkspaceAndStructuredErrors(t *testing.T) {
	t.Chdir(t.TempDir())

	code, stdout, stderr := runCLI("workspace", "init")
	if code != 0 || stderr != "" {
		t.Fatalf("workspace init: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	var success struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal([]byte(stdout), &success); err != nil || !success.OK {
		t.Fatalf("invalid success envelope: %v %s", err, stdout)
	}

	code, stdout, stderr = runCLI("task", "open", "--purpose", "Missing concurrency")
	if code != 2 || stdout != "" {
		t.Fatalf("invalid arguments must use exit 2 and empty stdout: code=%d stdout=%q", code, stdout)
	}
	var failure struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stderr), &failure); err != nil || failure.OK || failure.Error.Code != "INVALID_ARGUMENT" {
		t.Fatalf("invalid error envelope: %v %s", err, stderr)
	}

	code, stdout, stderr = runCLI("view", "status")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "tasks: []") || strings.Contains(stdout, `"ok"`) {
		t.Fatalf("Status must be a direct YAML View: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	entries, err := os.ReadDir(".norns/view")
	if err != nil || len(entries) != 1 || !strings.HasPrefix(entries[0].Name(), "status-") {
		t.Fatalf("Status View was not published: entries=%v err=%v", entries, err)
	}
}

func TestStructuredViewOutputSpillsAboveSixKiB(t *testing.T) {
	exact := bytes.Repeat([]byte("x"), structuredViewInlineLimit)
	var out bytes.Buffer
	if err := emitStructuredView(&out, exact, ".norns/view/status.yaml", false, false); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Bytes(), exact) {
		t.Fatal("View at the inline limit was not written directly")
	}

	out.Reset()
	large := append(exact, 'x')
	if err := emitStructuredView(&out, large, ".norns/view/trace.json", true, false); err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		OK   bool          `json:"ok"`
		Data publishedView `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("large View did not return a JSON envelope: %v\n%s", err, out.String())
	}
	if !envelope.OK || envelope.Data.Path != ".norns/view/trace.json" || envelope.Data.Format != "json" || envelope.Data.SizeBytes != len(large) {
		t.Fatalf("unexpected large View envelope: %#v", envelope)
	}
	if bytes.Contains(out.Bytes(), large) {
		t.Fatal("large View content leaked into stdout")
	}

	out.Reset()
	if err := emitStructuredView(&out, []byte(`{"small":true}`), ".norns/view/status.json", true, true); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil || !envelope.OK || envelope.Data.Path != ".norns/view/status.json" {
		t.Fatalf("forced View reference is invalid: %#v, %v", envelope, err)
	}
}

func TestCLIHelpTargetsNestedCommand(t *testing.T) {
	t.Chdir(t.TempDir())
	code, stdout, stderr := runCLI("help", "exploration")
	if code != 0 || stderr != "" {
		t.Fatalf("help failed: code=%d stderr=%s", code, stderr)
	}
	for _, command := range []string{"add", "assign", "finish", "retry", "running", "todo"} {
		if !strings.Contains(stdout, command) {
			t.Fatalf("exploration help omitted %q:\n%s", command, stdout)
		}
	}
	if strings.Contains(stdout, "workspace") {
		t.Fatalf("nested help unexpectedly rendered root help:\n%s", stdout)
	}
}

func TestCLIVisualLanguage(t *testing.T) {
	t.Chdir(t.TempDir())
	code, _, stderr := runCLI("workspace", "init")
	if code != 0 || stderr != "" {
		t.Fatalf("workspace init failed: code=%d stderr=%s", code, stderr)
	}
	code, stdout, stderr := runCLI("view", "visual", "--lang", "en")
	if code != 0 || stderr != "" {
		t.Fatalf("Visual View failed: code=%d stderr=%s", code, stderr)
	}
	var envelope struct {
		Data struct {
			Path string `json:"path"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("invalid Visual response: %v %s", err, stdout)
	}
	html, err := os.ReadFile(envelope.Data.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(html), `<html lang="en">`) {
		t.Fatal("--lang en did not generate an English document")
	}

	code, stdout, stderr = runCLI("view", "visual", "--lang", "de")
	if code != 2 || stdout != "" || !strings.Contains(stderr, `"code":"INVALID_ARGUMENT"`) {
		t.Fatalf("invalid --lang was not rejected: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestCLIVersionJSONFieldOrder(t *testing.T) {
	t.Chdir(t.TempDir())
	code, stdout, stderr := runCLI("version", "--json")
	if code != 0 || stderr != "" {
		t.Fatalf("version failed: code=%d stderr=%s", code, stderr)
	}
	want := []string{"product_version", "commit", "build_time", "protocol_version", "schema_version"}
	last := -1
	for _, field := range want {
		at := strings.Index(stdout, `"`+field+`"`)
		if at <= last {
			t.Fatalf("version fields are not stable: %s", stdout)
		}
		last = at
	}
}

func TestCLIHumanMessagesFollowChineseLocale(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	code, stdout, stderr := runCLI("workspace", "init")
	if code != 0 || stderr != "" {
		t.Fatalf("localized init failed: code=%d stderr=%s", code, stderr)
	}
	var envelope struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil || envelope.Message != "Workspace 已初始化" {
		t.Fatalf("success message was not localized: %v %s", err, stdout)
	}
	code, stdout, stderr = runCLI("task", "open", "--purpose", "x")
	if code != 2 || stdout != "" || !strings.Contains(stderr, "参数无效") {
		t.Fatalf("error message was not localized: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func runCLI(args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := Execute(args, strings.NewReader(""), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}
