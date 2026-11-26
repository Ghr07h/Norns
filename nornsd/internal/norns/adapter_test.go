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

func TestReadPublishedViewFormats(t *testing.T) {
	subjectPath := t.TempDir()
	viewRoot := filepath.Join(subjectPath, ".norns", "view")
	if err := os.MkdirAll(viewRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	for format, content := range map[ViewFormat]string{
		JSON:     `{"task":{"id":"task-1"},"graph":{"cognitions":[],"explorations":[]}}`,
		YAML:     "task:\n  id: task-1\ngraph:\n  cognitions: []\n  explorations: []\n",
		Markdown: "# Status View\n\n## Task\n\n> task-1\n",
	} {
		t.Run(string(format), func(t *testing.T) {
			path := filepath.Join(viewRoot, "status."+string(format))
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			envelope := []byte(`{"ok":true,"data":{"path":".norns/view/status.` + string(format) + `","format":"` + string(format) + `","size_bytes":72,"note":"large view written to file"}}`)
			published, filename, err := readPublishedView(subjectPath, envelope, "status", format)
			if err != nil || string(published) != content || filename != "status."+string(format) {
				t.Fatalf("published view = %s, %q, %v", published, filename, err)
			}
			if _, _, err := readPublishedView(subjectPath, envelope, "status", "wrong-format"); err == nil {
				t.Fatal("accepted mismatched format")
			}
		})
	}
}

func TestStructuredViewRejectsPublishedPathOutsideViewDirectory(t *testing.T) {
	subjectPath := t.TempDir()
	outside := filepath.Join(subjectPath, "outside.json")
	if err := os.WriteFile(outside, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := readPublishedView(subjectPath, []byte(`{"ok":true,"data":{"path":"outside.json","format":"json"}}`), "trace", JSON)
	if err == nil {
		t.Fatal("structuredView accepted a path outside .norns/view")
	}
}

func TestPublishedViewRejectsSymlinkDirectoryEscape(t *testing.T) {
	subject := t.TempDir()
	view := filepath.Join(subject, ".norns", "view")
	outside := t.TempDir()
	if err := os.MkdirAll(view, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "trace.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(view, "linked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := resolveViewPath(subject, filepath.Join(view, "linked", "trace.json")); err == nil {
		t.Fatal("followed a directory symlink outside the View directory")
	}
}
