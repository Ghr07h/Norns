package eventlog

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ghr07h/norns/nornsd/internal/domain"
)

func TestReadablePagesAndUpdateCursor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "readable.jsonl")
	for seq := uint64(1); seq <= 8; seq++ {
		if err := appendJSONLine(path, domain.Event{Seq: seq, Type: "test", Timestamp: time.Unix(int64(seq), 0).UTC(), Source: "test", Payload: []byte(`{"seq":1}`)}); err != nil {
			t.Fatal(err)
		}
	}

	latest, err := ReadableLatest(path, 5)
	if err != nil || len(latest.Events) != 5 || latest.Events[0].Seq != 4 || latest.Events[4].Seq != 8 || latest.Older == nil || !latest.HasMore {
		t.Fatalf("latest=%#v err=%v", latest, err)
	}
	older, err := ReadableBefore(path, *latest.Older, 2)
	if err != nil || len(older.Events) != 2 || older.Events[0].Seq != 2 || older.Events[1].Seq != 3 || older.Older == nil || !older.HasMore {
		t.Fatalf("older=%#v err=%v", older, err)
	}
	oldest, err := ReadableBefore(path, *older.Older, 5)
	if err != nil || len(oldest.Events) != 1 || oldest.Events[0].Seq != 1 || oldest.Older != nil || oldest.HasMore {
		t.Fatalf("oldest=%#v err=%v", oldest, err)
	}
	updated, err := ReadableHasUpdates(path, latest.Cursor)
	if err != nil || updated {
		t.Fatalf("updates=%v err=%v", updated, err)
	}
	if err = appendJSONLine(path, domain.Event{Seq: 9, Type: "test", Timestamp: time.Unix(9, 0).UTC(), Source: "test", Payload: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	updated, err = ReadableHasUpdates(path, latest.Cursor)
	if err != nil || !updated {
		t.Fatalf("updates=%v err=%v", updated, err)
	}
	after, err := ReadableAfter(path, latest.Cursor, 5)
	if err != nil || len(after.Events) != 1 || after.Events[0].Seq != 9 || after.HasMore {
		t.Fatalf("after=%#v err=%v", after, err)
	}
}

func TestReadableIgnoresPartialTailAndRejectsStaleCursor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "readable.jsonl")
	if err := appendJSONLine(path, domain.Event{Seq: 1, Type: "test", Timestamp: time.Now().UTC(), Source: "test", Payload: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	page, err := ReadableLatest(path, 5)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.WriteString(`{"seq":2`); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	updated, err := ReadableHasUpdates(path, page.Cursor)
	if err != nil || updated {
		t.Fatalf("partial update=%v err=%v", updated, err)
	}
	if _, err = ReadableAfter(path, ReadableCursor{Offset: page.Cursor.Offset + 1, Seq: 1}, 5); !errors.Is(err, ErrInvalidReadableCursor) {
		t.Fatalf("expected invalid cursor, got %v", err)
	}
	if err = os.Truncate(path, 0); err != nil {
		t.Fatal(err)
	}
	if err = appendJSONLine(path, domain.Event{Seq: 1, Type: "changed", Timestamp: time.Now().UTC(), Source: "test", Payload: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err = ReadableHasUpdates(path, page.Cursor); !errors.Is(err, ErrInvalidReadableCursor) {
		t.Fatalf("expected rebuilt file to invalidate cursor, got %v", err)
	}
}
