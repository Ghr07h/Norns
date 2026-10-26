package norns

import (
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Ghr07h/norns/nornsd/internal/domain"
)

func TestVisualArgs(t *testing.T) {
	for _, test := range []struct {
		lang string
		want []string
	}{
		{"", []string{"view", "visual"}},
		{"zh", []string{"view", "visual", "--lang", "zh"}},
		{"en", []string{"view", "visual", "--lang", "en"}},
	} {
		got, err := visualArgs(test.lang)
		if err != nil || !reflect.DeepEqual(got, test.want) {
			t.Fatalf("visualArgs(%q) = %#v, %v; want %#v", test.lang, got, err, test.want)
		}
	}
	if _, err := visualArgs("de"); err == nil {
		t.Fatal("visualArgs accepted an unsupported language")
	}
}

func TestTranslateCommandErrorPreservesStructuredFailure(t *testing.T) {
	err := translateCommandError(&commandError{ExitCode: 1, Stderr: []byte(`{"ok":false,"error":{"code":"WORKSPACE_NOT_INITIALIZED","message":"Workspace is not initialized","details":{"action":"task open"}}}`)})
	got := domain.AsError(err)
	if got.Code != "WORKSPACE_NOT_INITIALIZED" || got.HTTPStatus != http.StatusConflict || got.Message != "Workspace is not initialized" {
		t.Fatalf("translated error = %#v", got)
	}
	if got.Details["action"] != "task open" {
		t.Fatalf("translated details = %#v", got.Details)
	}
}

func TestStructuredViewAcceptsInlineAndPublishedJSON(t *testing.T) {
	subjectPath := t.TempDir()
	viewRoot := filepath.Join(subjectPath, ".norns", "view")
	if err := os.MkdirAll(viewRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	content := []byte(`{"task":{"id":"task-1"},"graph":{"cognitions":[],"explorations":[]}}`)
	inline, inlineName, err := structuredView(subjectPath, content, "status")
	if err != nil || string(inline) != string(content) || inlineName != "" {
		t.Fatalf("inline view = %s, %q, %v", inline, inlineName, err)
	}
	path := filepath.Join(viewRoot, "status.json")
	if err = os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	envelope := []byte(`{"ok":true,"data":{"path":".norns/view/status.json","format":"json","size_bytes":72}}`)
	published, filename, err := structuredView(subjectPath, envelope, "status")
	if err != nil || string(published) != string(content) || filename != "status.json" {
		t.Fatalf("published view = %s, %q, %v", published, filename, err)
	}
}

func TestStructuredViewRejectsPublishedPathOutsideViewDirectory(t *testing.T) {
	subjectPath := t.TempDir()
	outside := filepath.Join(subjectPath, "outside.json")
	if err := os.WriteFile(outside, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := structuredView(subjectPath, []byte(`{"ok":true,"data":{"path":"outside.json"}}`), "trace")
	if err == nil {
		t.Fatal("structuredView accepted a path outside .norns/view")
	}
}
