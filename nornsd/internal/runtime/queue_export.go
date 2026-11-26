package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	storesqlite "github.com/Ghr07h/norns/nornsd/internal/store/sqlite"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"time"
)

type queueExport struct {
	ID          string                     `json:"export_id"`
	RootID      string                     `json:"root_id"`
	GeneratedAt time.Time                  `json:"generated_at"`
	Reason      string                     `json:"reason"`
	Templates   map[string]string          `json:"templates"`
	Records     []storesqlite.QueuedRecord `json:"records"`
}

func (m *Manager) ExportQueued(ctx context.Context, reason string) error {
	return m.store.ExportQueued(ctx, reason, func(records []storesqlite.QueuedRecord) error {
		templates := map[string]string{}
		files, err := os.ReadDir(filepath.Join(m.owner.Control, "configs"))
		if err != nil {
			return err
		}
		for _, file := range files {
			if !file.IsDir() && strings.HasSuffix(file.Name(), ".yaml") {
				b, err := os.ReadFile(filepath.Join(m.owner.Control, "configs", file.Name()))
				if err != nil {
					return err
				}
				templates[file.Name()] = string(b)
			}
		}
		id, err := queueExportID(m.owner.Identity.RootID, templates, records)
		if err != nil {
			return err
		}
		dir := filepath.Join(m.owner.Control, "queue_record")
		if err = os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		// A crash can occur after rename but before the SQLite commit. Reuse the
		// durable export rather than emitting duplicate records on every restart.
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				return err
			}
			var old queueExport
			if json.Unmarshal(b, &old) == nil && old.ID == id {
				verified, verifyErr := queueExportID(old.RootID, old.Templates, old.Records)
				if verifyErr != nil || verified != id {
					return fmt.Errorf("queue export integrity check failed")
				}
				if m.onQueueExport != nil {
					m.onQueueExport(filepath.Join(dir, entry.Name()), len(records))
				}
				return nil
			}
		}
		output := queueExport{id, m.owner.Identity.RootID, time.Now().UTC(), reason, templates, records}
		data, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')
		file, err := os.CreateTemp(dir, ".export-*.tmp")
		if err != nil {
			return err
		}
		temp := file.Name()
		defer os.Remove(temp)
		if err = file.Chmod(0600); err == nil {
			_, err = file.Write(data)
		}
		if err == nil {
			err = file.Sync()
		}
		closeErr := file.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		path := filepath.Join(dir, output.GeneratedAt.Format("20060102T150405.000000000Z")+".json")
		if _, err = os.Lstat(path); err == nil {
			return fmt.Errorf("queue export path already exists")
		}
		if !os.IsNotExist(err) {
			return err
		}
		if err = os.Rename(temp, path); err != nil {
			return err
		}
		// Verify the published bytes before committing cancellation.
		saved, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(saved, data) {
			return fmt.Errorf("queue export verification failed")
		}
		// Windows does not support FlushFileBuffers on directory handles.
		// The payload is already flushed before publication on every platform.
		if goruntime.GOOS != "windows" {
			directory, err := os.Open(dir)
			if err != nil {
				return err
			}
			syncErr := directory.Sync()
			closeErr = directory.Close()
			if syncErr != nil {
				return syncErr
			}
			if closeErr != nil {
				return closeErr
			}
		}

		if m.onQueueExport != nil {
			m.onQueueExport(path, len(records))
		}
		return nil
	})
}

func queueExportID(rootID string, templates map[string]string, records []storesqlite.QueuedRecord) (string, error) {
	payload, err := json.Marshal(struct {
		RootID    string
		Templates map[string]string
		Records   []storesqlite.QueuedRecord
	}{rootID, templates, records})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}
