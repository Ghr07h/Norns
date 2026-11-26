package archive

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchiveIncludesHiddenFilesSymlinksAndSQLiteWAL(t *testing.T) {
	root := t.TempDir()
	for rel, content := range map[string]string{
		".nornsd/service/root.json":                   "identity",
		".nornsd/service/nornsd_lock":                 "locked",
		".nornsd/service/owner.json":                  "owner",
		".nornsd/queue_record/.export-unfinished.tmp": "temporary",
		".nornsd/staging/unfinished":                  "temporary",
		".nornsd/subjects/subject/logs/raw.jsonl":     "raw log\n",
		".nornsd/subjects/subject/staging/unfinished": "temporary",
		"subject/.norns.lock":                         "locked",
		"subject/.norns/view/status.md":               "# Status View\n",
		"subject/result.txt":                          "finding",
	} {
		filename := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, rel := range []string{".nornsd/service/nornsd.sqlite", "subject/.norns/norns.sqlite"} {
		db, err := sql.Open("sqlite", filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		for _, query := range []string{`PRAGMA journal_mode=WAL`, `PRAGMA wal_autocheckpoint=0`, `PRAGMA user_version=13`, `CREATE TABLE finding (id INTEGER PRIMARY KEY, content TEXT)`, `INSERT INTO finding VALUES (1,'uncheckpointed finding')`} {
			if _, err := db.Exec(query); err != nil {
				t.Fatal(err)
			}
		}
	}
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("must not dereference"), 0600); err != nil {
		t.Fatal(err)
	}
	linkCreated := os.Symlink(outside, filepath.Join(root, "subject", "external-link")) == nil
	file, err := Create(context.Background(), root, root, "root-test")
	if err != nil {
		t.Fatal(err)
	}
	temp := file.directory
	if rel, _ := filepath.Rel(root, temp); rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		t.Fatalf("archive is inside source: %s", temp)
	}
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	entries := make(map[string][]byte)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		rel := strings.TrimPrefix(header.Name, filepath.Base(root)+"/")
		body, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		entries[rel] = body
		if rel == "subject/external-link" && (header.Typeflag != tar.TypeSymlink || header.Linkname != outside || len(body) != 0) {
			t.Fatalf("invalid archived link: %#v", header)
		}
	}
	_ = gz.Close()
	for _, rel := range []string{".nornsd/service/root.json", ".nornsd/subjects/subject/logs/raw.jsonl", "subject/.norns/view/status.md", "subject/result.txt"} {
		if _, ok := entries[rel]; !ok {
			t.Errorf("missing file %s", rel)
		}
	}
	if linkCreated {
		if _, ok := entries["subject/external-link"]; !ok {
			t.Error("missing symlink")
		}
	}
	for rel, body := range entries {
		if strings.HasSuffix(rel, "-wal") || strings.HasSuffix(rel, "-shm") || strings.HasSuffix(rel, "/nornsd_lock") || strings.Contains(rel, "unfinished") || strings.HasSuffix(rel, ".norns.lock") || rel == ".nornsd/service/owner.json" {
			t.Errorf("included transient entry %s", rel)
		}
		if strings.HasSuffix(rel, ".sqlite") {
			filename := filepath.Join(t.TempDir(), "snapshot.sqlite")
			if err := os.WriteFile(filename, body, 0600); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", filename)
			if err != nil {
				t.Fatal(err)
			}
			var content, integrity string
			var version int
			if err := db.QueryRow(`SELECT content FROM finding WHERE id=1`).Scan(&content); err != nil || content != "uncheckpointed finding" {
				t.Fatalf("WAL data missing from %s: %q, %v", rel, content, err)
			}
			if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
				t.Fatalf("snapshot integrity: %q, %v", integrity, err)
			}
			if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 13 {
				t.Fatalf("snapshot schema: %d, %v", version, err)
			}
			_ = db.Close()
		}
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(temp); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary directory not removed: %v", err)
	}
}

func TestArchiveCancellationAndFailureCleanTemporaryFiles(t *testing.T) {
	temp := t.TempDir()
	t.Setenv("TMPDIR", temp)
	t.Setenv("TMP", temp)
	t.Setenv("TEMP", temp)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".norns"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".norns", "norns.sqlite"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(context.Background(), root, root, "broken"); err == nil {
		t.Fatal("accepted corrupt database")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Create(ctx, root, root, "cancelled"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	entries, err := os.ReadDir(temp)
	if err != nil || len(entries) != 0 {
		t.Fatalf("leaked temporary files: %v, %v", entries, err)
	}
}
