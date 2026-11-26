package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCognitionMetaPersistsAcrossOriginsWithoutStructuredViewChanges(t *testing.T) {
	ctx := context.Background()
	s, _, root := protocolWorkspace(t)
	exp := addExplorationForTest(t, s, root, "Geometry")
	encoded := `{"type":"theorem","scope":"geometry","note":"</script><img src=x onerror=alert(1)>"}`
	want := map[string]string{"type": "theorem", "scope": "geometry", "note": "</script><img src=x onerror=alert(1)>"}
	ids := map[string]bool{}
	for _, req := range []AddCognitionRequest{
		{Title: "Global", Content: "Global assertion", Origin: "user", Meta: &encoded},
		{Title: "Hint", Content: "Exploration assertion", Origin: "user", ExplorationID: &exp, Meta: &encoded},
		{Title: "Theorem", Content: "Derived with evidence", Origin: "exploration", ExplorationID: &exp, Meta: &encoded},
	} {
		r, err := s.AddCognition(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		ids[resultID(t, r)] = true
		if !reflect.DeepEqual(r.Data.(map[string]any)["meta"], want) {
			t.Fatal("creation result lost metadata")
		}
	}
	db, err := sql.Open("sqlite", s.Paths.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var initMeta string
	if err := db.QueryRow(`SELECT meta FROM cognitions WHERE id=?`, root).Scan(&initMeta); err != nil || initMeta != "{}" {
		t.Fatalf("Task-init must default to empty metadata: %q %v", initMeta, err)
	}
	for id := range ids {
		var stored string
		var meta map[string]string
		if err := db.QueryRow(`SELECT meta FROM cognitions WHERE id=?`, id).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(stored), &meta); err != nil || !reflect.DeepEqual(meta, want) {
			t.Fatalf("stored metadata changed: %s %v", stored, err)
		}
	}
	status, _, err := s.StatusView(ctx, nil, ViewJSON)
	if err != nil {
		t.Fatal(err)
	}
	trace, _, err := s.TraceView(ctx, exp, ViewYAML)
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range [][]byte{status, trace} {
		if strings.Contains(string(view), "meta") || strings.Contains(string(view), "theorem") {
			t.Fatalf("metadata leaked into structured View: %s", view)
		}
	}
	visual, err := s.VisualView(ctx, false, "en")
	if err != nil {
		t.Fatal(err)
	}
	html, err := os.ReadFile(filepath.Join(s.Paths.Root, visual.Data.(map[string]any)["path"].(string)))
	if err != nil {
		t.Fatal(err)
	}
	page := string(html)
	if strings.Contains(page, want["note"]) {
		t.Fatal("metadata broke out of the embedded JSON script")
	}
	start := strings.Index(page, `<script type="application/json" id="norns-data">`)
	if start < 0 {
		t.Fatal("Visual omitted its data")
	}
	data := page[start+len(`<script type="application/json" id="norns-data">`):]
	data = data[:strings.Index(data, "</script>")]
	var payload visualModel
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		t.Fatal(err)
	}
	for _, cog := range payload.Cognitions {
		if ids[cog.ID] {
			if !reflect.DeepEqual(cog.Meta, want) {
				t.Fatalf("Visual lost metadata for %s", cog.ID)
			}
			delete(ids, cog.ID)
		}
	}
	if len(ids) != 0 {
		t.Fatal("Visual omitted cognitions with metadata")
	}
}

func TestInvalidCognitionMetaDoesNotAppendHint(t *testing.T) {
	s, _, root := protocolWorkspace(t)
	exp := addExplorationForTest(t, s, root, "A")
	for _, encoded := range []string{"", "null", "[]", `{"type":null}`, `{"type":1}`, `{"type":{}}`} {
		_, err := s.AddCognition(context.Background(), AddCognitionRequest{Title: "Invalid", Content: "Must not persist", Origin: "user", ExplorationID: &exp, Meta: &encoded})
		if errorCode(err) != "INVALID_ARGUMENT" {
			t.Fatalf("wrong error for %q: %v", encoded, err)
		}
	}
	view := decodeStatus(t, s, nil)
	if len(view.Graph.Cognitions) != 1 || len(view.Graph.Explorations[0].Hints) != 0 {
		t.Fatal("invalid metadata partially wrote a Cognition or hint")
	}
}

func TestPersistedCognitionMetaRejectsCorruption(t *testing.T) {
	for _, encoded := range []string{"{", "null", "[]", `{"type":null}`, `{"type":1}`} {
		t.Run(encoded, func(t *testing.T) {
			s, _, root := protocolWorkspace(t)
			db, err := sql.Open("sqlite", s.Paths.Database)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.Exec(`PRAGMA ignore_check_constraints=ON`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`UPDATE cognitions SET meta=? WHERE id=?`, encoded, root); err != nil {
				t.Fatal(err)
			}
			if _, _, err := s.StatusView(context.Background(), nil, ViewJSON); errorCode(err) != "WORKSPACE_STATE_UNTRUSTED" {
				t.Fatalf("metadata corruption was not rejected: %v", err)
			}
		})
	}
}
