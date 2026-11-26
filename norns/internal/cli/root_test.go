package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ghr07h/norns/norns/internal/app"
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

func TestUninitializedDirectoryExplainsWorkspaceLocation(t *testing.T) {
	for _, locale := range []string{"en_US.UTF-8", "zh_CN.UTF-8"} {
		t.Run(locale, func(t *testing.T) {
			t.Setenv("LC_ALL", locale)
			workspace := t.TempDir()
			t.Chdir(workspace)
			if code, _, stderr := runCLI("workspace", "init"); code != 0 {
				t.Fatalf("workspace init: code=%d stderr=%s", code, stderr)
			}
			subdirectory := filepath.Join(workspace, "exploration")
			if err := os.Mkdir(subdirectory, 0o700); err != nil {
				t.Fatal(err)
			}
			for _, directory := range []string{subdirectory, t.TempDir()} {
				t.Chdir(directory)
				for _, command := range [][]string{{"view", "status"}, {"workspace", "reset"}, {"workspace", "check"}, {"workspace", "free"}, {"task", "list"}} {
					code, stdout, stderr := runCLI(command...)
					if code != 1 || stdout != "" {
						t.Fatalf("%v: code=%d stdout=%q stderr=%s", command, code, stdout, stderr)
					}
					var failure struct {
						OK    bool `json:"ok"`
						Error struct {
							Code    string `json:"code"`
							Message string `json:"message"`
						} `json:"error"`
					}
					if err := json.Unmarshal([]byte(stderr), &failure); err != nil {
						t.Fatal(err)
					}
					if failure.OK || failure.Error.Code != "WORKSPACE_NOT_INITIALIZED" {
						t.Fatalf("unexpected error envelope: %s", stderr)
					}
					fragments := []string{"Workspace marker", "Check whether pwd is the Subject directory", "does not search parent directories", "must be run from the Subject directory", "only when explicitly creating a new, independent Workspace"}
					if strings.HasPrefix(locale, "zh") {
						fragments = []string{"Workspace 标记", "请检查 pwd 是否为 Subject 目录", "不会向上查找父级目录", "仅可在 Subject 目录执行", "在明确需要创建新的、独立的 Workspace 时使用 norns workspace init 命令"}
					}
					for _, fragment := range fragments {
						if !strings.Contains(failure.Error.Message, fragment) {
							t.Fatalf("message omitted %q: %s", fragment, failure.Error.Message)
						}
					}
				}
				if _, err := os.Stat(filepath.Join(directory, ".norns")); !os.IsNotExist(err) {
					t.Fatalf("failed commands must not initialize a Workspace: %v", err)
				}
				if _, err := os.Stat(filepath.Join(directory, ".norns.lock")); !os.IsNotExist(err) {
					t.Fatalf("failed commands must not create a lifecycle lock: %v", err)
				}
			}
			t.Chdir(workspace)
			if code, _, stderr := runCLI("view", "status"); code != 0 {
				t.Fatalf("original Workspace is no longer usable: code=%d stderr=%s", code, stderr)
			}
		})
	}
}

func TestStructuredViewOutputSpillsAboveSixKiB(t *testing.T) {
	t.Setenv("LC_ALL", "en_US.UTF-8")
	exact := bytes.Repeat([]byte("x"), structuredViewInlineLimit)
	var out bytes.Buffer
	if err := emitStructuredView(&out, exact, ".norns/view/status.yaml", app.ViewYAML, false); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Bytes(), exact) {
		t.Fatal("View at the inline limit was not written directly")
	}

	out.Reset()
	large := append(exact, 'x')
	if err := emitStructuredView(&out, large, ".norns/view/trace.json", app.ViewJSON, false); err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		OK   bool          `json:"ok"`
		Data publishedView `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("large View did not return a JSON envelope: %v\n%s", err, out.String())
	}
	if !envelope.OK || envelope.Data.Path != ".norns/view/trace.json" || envelope.Data.Format != "json" || envelope.Data.SizeBytes != len(large) || !strings.Contains(envelope.Data.Note, "exceeds the inline output limit") || !strings.Contains(envelope.Data.Note, "file at path") {
		t.Fatalf("unexpected large View envelope: %#v", envelope)
	}
	if bytes.Contains(out.Bytes(), large) {
		t.Fatal("large View content leaked into stdout")
	}

	out.Reset()
	if err := emitStructuredView(&out, []byte(`{"small":true}`), ".norns/view/status.json", app.ViewJSON, true); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil || !envelope.OK || envelope.Data.Path != ".norns/view/status.json" || !strings.Contains(envelope.Data.Note, "a file reference was requested") {
		t.Fatalf("forced View reference is invalid: %#v, %v", envelope, err)
	}
}

func TestCLIHelpTargetsNestedCommand(t *testing.T) {
	t.Chdir(t.TempDir())
	code, stdout, stderr := runCLI("help", "exploration")
	if code != 0 || stderr != "" {
		t.Fatalf("help failed: code=%d stderr=%s", code, stderr)
	}
	for _, command := range []string{"add", "get", "assign", "finish", "retry", "running", "todo"} {
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

func TestCLICognitionMetaValidationAndResult(t *testing.T) {
	t.Chdir(t.TempDir())
	args := []string{"cognition", "add", "--origin", "user", "--cognition-title", "Theorem", "--cognition-content", "Geometry"}
	for _, value := range []string{"", "null", "[]", `{"type":null}`, `{"type":1}`} {
		code, stdout, stderr := runCLI(append(args, "--meta", value)...)
		if code != 2 || stdout != "" || !strings.Contains(stderr, "INVALID_ARGUMENT") {
			t.Fatalf("invalid meta %q: code=%d stdout=%s stderr=%s", value, code, stdout, stderr)
		}
	}
	if _, err := os.Stat(".norns"); !os.IsNotExist(err) {
		t.Fatalf("invalid metadata initialized a Workspace: %v", err)
	}
	if code, _, stderr := runCLI("task", "open", "--purpose", "Math", "--concurrency", "1", "--info", "Geometry"); code != 0 {
		t.Fatal(stderr)
	}
	code, stdout, stderr := runCLI(append(args, "--meta", `{"type":"theorem","scope":"geometry"}`)...)
	if code != 0 || stderr != "" {
		t.Fatalf("valid meta: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	var result struct {
		Data struct {
			Meta map[string]string `json:"meta"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil || len(result.Data.Meta) != 2 || result.Data.Meta["type"] != "theorem" || result.Data.Meta["scope"] != "geometry" {
		t.Fatalf("CLI lost metadata: %s %v", stdout, err)
	}
}

func runCLI(args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := Execute(args, strings.NewReader(""), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestStructuredViewFileNoteFollowsLocale(t *testing.T) {
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	var out bytes.Buffer
	if err := emitStructuredView(&out, bytes.Repeat([]byte("x"), structuredViewInlineLimit+1), ".norns/view/status.yaml", app.ViewYAML, false); err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Data publishedView `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil || !strings.Contains(envelope.Data.Note, "完整内容已写入 path 指向的文件") {
		t.Fatalf("missing localized note: %v %s", err, out.String())
	}
}

func TestCLIGetEntities(t *testing.T) {
	t.Chdir(t.TempDir())
	run := func(args ...string) map[string]any {
		t.Helper()
		code, stdout, stderr := runCLI(args...)
		var envelope struct {
			OK   bool           `json:"ok"`
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal([]byte(stdout), &envelope); err != nil || code != 0 || stderr != "" || !envelope.OK {
			t.Fatalf("%v: %d %s %s %v", args, code, stdout, stderr, err)
		}
		return envelope.Data
	}
	run("task", "open", "--purpose", "Get details", "--concurrency", "1", "--info", "Subject")
	cog := run("cognition", "add", "--origin", "user", "--cognition-title", "Metadata", "--cognition-content", "Full content", "--meta", `{"type":"theorem"}`)
	cogID := cog["id"].(string)
	detail := run("cognition", "get", "--cognition", cogID)
	if detail["title"] != "Metadata" || detail["content"] != "Full content" || detail["meta"].(map[string]any)["type"] != "theorem" {
		t.Fatalf("get lost Cognition details: %#v", detail)
	}
	exp := run("exploration", "add", "--exploration-title", "Inspect", "--exploration-content", "Inspect metadata", "--source-cognitions", cogID)
	expID := exp["id"].(string)
	detail = run("exploration", "get", "--exploration", expID)
	if detail["status"] != "todo" || detail["title"] != "Inspect" || detail["sources"].([]any)[0] != cogID || len(detail["hints"].([]any)) != 0 {
		t.Fatalf("get lost Exploration details: %#v", detail)
	}
	for _, args := range [][]string{
		{"cognition", "get"}, {"exploration", "get"},
		{"cognition", "get", "--cognition", expID},
		{"exploration", "get", "--exploration", cogID},
		{"cognition", "get", "--cognition", cogID, "extra"},
	} {
		code, stdout, stderr := runCLI(args...)
		if code != 2 || stdout != "" || !strings.Contains(stderr, `"code":"INVALID_ARGUMENT"`) {
			t.Fatalf("invalid get arguments accepted: %v %d %s %s", args, code, stdout, stderr)
		}
	}
}

func TestCLIMarkdownViewsAndFileReferences(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("LC_ALL", "C")
	run := func(args ...string) string {
		t.Helper()
		code, stdout, stderr := runCLI(args...)
		if code != 0 || stderr != "" {
			t.Fatalf("%v: %d %s %s", args, code, stdout, stderr)
		}
		return stdout
	}
	run("task", "open", "--purpose", "Read Markdown", "--concurrency", "1", "--info", "Subject")
	exp := binaryEntityID(t, []byte(run("exploration", "add", "--exploration-title", "Inspect", "--exploration-content", "Inspect subject")))
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"Status", []string{"view", "status"}},
		{"Trace", []string{"view", "trace", "--exploration", exp}},
	} {
		args := append(append([]string{}, tc.args...), "--md")
		content := run(args...)
		if !strings.HasPrefix(content, "# "+tc.name+" View\n\n") {
			t.Fatalf("Markdown not inline: %s", content)
		}
		files, err := filepath.Glob(".norns/view/*.md")
		if err != nil || len(files) == 0 {
			t.Fatal("Markdown was not published")
		}
		var reference struct {
			Data publishedView `json:"data"`
		}
		if err := json.Unmarshal([]byte(run(append(args, "--output-reference")...)), &reference); err != nil || reference.Data.Format != "md" || !strings.HasSuffix(reference.Data.Path, ".md") {
			t.Fatalf("invalid forced Markdown reference: %#v %v", reference, err)
		}
		if file, err := os.ReadFile(reference.Data.Path); err != nil || len(file) != reference.Data.SizeBytes || !strings.HasPrefix(string(file), "# "+tc.name+" View\n\n") {
			t.Fatalf("Markdown file differs: %v", err)
		}
		code, stdout, stderr := runCLI(append(args, "--json")...)
		if code != 2 || stdout != "" || !strings.Contains(stderr, `"code":"INVALID_ARGUMENT"`) {
			t.Fatalf("conflicting formats accepted: %d %s %s", code, stdout, stderr)
		}
	}
	large := strings.Repeat("Knowledge ", 800)
	run("cognition", "add", "--origin", "user", "--cognition-title", "Long", "--cognition-content", large)
	for _, args := range [][]string{{"view", "status", "--md"}, {"view", "trace", "--exploration", exp, "--md"}} {
		payload := run(args...)
		var reference struct {
			Data publishedView `json:"data"`
		}
		if err := json.Unmarshal([]byte(payload), &reference); err != nil || reference.Data.Format != "md" || reference.Data.SizeBytes <= structuredViewInlineLimit || reference.Data.Note == "" {
			t.Fatalf("invalid large Markdown reference: %v %s", err, payload)
		}
		content, err := os.ReadFile(reference.Data.Path)
		if err != nil || len(content) != reference.Data.SizeBytes || !strings.Contains(string(content), strings.TrimSpace(large)) {
			t.Fatalf("large Markdown file incomplete: %v", err)
		}
	}
}
