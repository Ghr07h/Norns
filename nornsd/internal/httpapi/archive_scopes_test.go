package httpapi

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ghr07h/norns/nornsd/internal/domain"
	"github.com/Ghr07h/norns/nornsd/internal/root"
	"github.com/google/uuid"
)

func TestUnifiedArchiveFiveScopesAndInvalidCombinations(t *testing.T) {
	server, subject, workspace := newArchiveTestServer(t, nil)
	owner := server.service.Owner
	ctx := context.Background()
	rid := uuid.NewString()
	if _, err := owner.Store.CreateDirectRuntime(ctx, subject.ID, rid, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := owner.Store.SetRuntimeTerminal(ctx, subject.ID, rid, domain.RuntimeEnded, "test", "", time.Now(), true); err != nil {
		t.Fatal(err)
	}
	runtimeDir := root.RuntimeDataPath(workspace, rid)
	if err := os.MkdirAll(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	for path, value := range map[string]string{filepath.Join(workspace, "work.txt"): "work", filepath.Join(runtimeDir, "launch.json"): "{}", filepath.Join(owner.Path, "root-extra.txt"): "extra", filepath.Join(owner.SubjectData(subject.ID), "staging", "temp.txt"): "skip"} {
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		query                   string
		work, launch, db, extra bool
	}{
		{"scope=subject", true, true, true, true},
		{"scope=subject&subject_id=" + subject.ID, true, false, false, false},
		{"scope=rundata", false, true, true, false},
		{"scope=rundata&subject_id=" + subject.ID, false, true, false, false},
		{"scope=rundata&subject_id=" + subject.ID + "&runtime_id=" + rid, false, true, false, false},
	} {
		t.Run(tc.query, func(t *testing.T) {
			response := authenticatedRequest(t, server, http.MethodGet, "/api/archive?"+tc.query)
			if response.Code != 200 {
				t.Fatalf("%d %s", response.Code, response.Body.String())
			}
			gz, err := gzip.NewReader(bytes.NewReader(response.Body.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			defer gz.Close()
			tr := tar.NewReader(gz)
			work, launch, db, extra := false, false, false, false
			for {
				h, err := tr.Next()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				switch filepath.Base(h.Name) {
				case "work.txt":
					work = true
				case "launch.json":
					launch = true
				case "nornsd.sqlite":
					db = true
				case "root-extra.txt":
					extra = true
				case "temp.txt", "nornsd_lock", "owner.json", "nornsd.sqlite-wal", "nornsd.sqlite-shm":
					t.Fatalf("temporary item archived: %s", h.Name)
				}
			}
			if work != tc.work || launch != tc.launch || db != tc.db || extra != tc.extra {
				t.Fatalf("wrong scope: work=%v launch=%v db=%v extra=%v", work, launch, db, extra)
			}
		})
	}
	other, err := server.service.CreateSubject(ctx, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"", "scope=bad", "scope=subject&runtime_id=" + rid, "scope=rundata&runtime_id=" + rid, "scope=rundata&subject_id=../escape", "scope=rundata&subject_id=" + subject.ID + "&runtime_id=bad"} {
		r := authenticatedRequest(t, server, "GET", "/api/archive?"+query)
		if r.Code != 400 {
			t.Fatalf("invalid %s: %d %s", query, r.Code, r.Body.String())
		}
	}
	r := authenticatedRequest(t, server, "GET", "/api/archive?scope=rundata&subject_id="+other.ID+"&runtime_id="+rid)
	if r.Code != 404 {
		t.Fatalf("cross Subject runtime leaked: %d %s", r.Code, r.Body.String())
	}
	for _, route := range []string{"/api/subjects/" + subject.ID + "/archive", "/api/control/archive"} {
		r := authenticatedRequest(t, server, "GET", route)
		if r.Code != 404 {
			t.Fatalf("obsolete route %s: %d", route, r.Code)
		}
	}
}
