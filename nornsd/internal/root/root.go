package root

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
	"github.com/google/uuid"

	"github.com/Ghr07h/norns/nornsd/internal/domain"
	storesqlite "github.com/Ghr07h/norns/nornsd/internal/store/sqlite"
	"github.com/Ghr07h/norns/nornsd/internal/version"
)

type Identity struct {
	FormatVersion  int       `json:"format_version"`
	RootID         string    `json:"root_id"`
	DatabaseSchema int       `json:"database_schema"`
	CreatedAt      time.Time `json:"created_at"`
}

type Owner struct {
	Path     string
	Control  string
	lock     *flock.Flock
	Store    *storesqlite.Store
	Identity Identity
}

func Acquire(ctx context.Context, path string, mode string, now time.Time) (*Owner, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, domain.Wrap("INVALID_REQUEST", "invalid workspace root", http.StatusBadRequest, err)
	}
	control := filepath.Join(abs, ".nornsd_root")
	if err = os.MkdirAll(control, 0700); err != nil {
		return nil, domain.Wrap("SERVICE_UNAVAILABLE", "cannot create nornsd root", http.StatusServiceUnavailable, err)
	}
	lock := flock.New(filepath.Join(control, "lock"))
	ok, err := lock.TryLockContext(ctx, 0)
	if err != nil {
		return nil, domain.Wrap("ROOT_IN_USE", "cannot lock nornsd root", http.StatusConflict, err)
	}
	if !ok {
		return nil, domain.NewError("ROOT_IN_USE", "nornsd root is already in use", http.StatusConflict)
	}
	owner := &Owner{Path: abs, Control: control, lock: lock}
	cleanup := func(e error) (*Owner, error) { _ = lock.Unlock(); return nil, e }
	identityPath := filepath.Join(control, "root.json")
	identity, exists, err := readIdentity(identityPath)
	if err != nil {
		return cleanup(err)
	}
	if !exists {
		if hasSubjectIdentity(abs) {
			return cleanup(domain.NewError("UNSUPPORTED_DATABASE", "nornsd database is missing while Subject identities exist", http.StatusInternalServerError))
		}
		identity = Identity{FormatVersion: 1, RootID: uuid.NewString(), DatabaseSchema: version.DatabaseSchema, CreatedAt: now.UTC()}
	}
	databasePath := filepath.Join(control, "nornsd.sqlite")
	if exists {
		if _, statErr := os.Stat(databasePath); errors.Is(statErr, os.ErrNotExist) {
			return cleanup(domain.NewError("UNSUPPORTED_DATABASE", "nornsd database is missing", http.StatusInternalServerError))
		}
	}
	store, created, err := storesqlite.Open(ctx, databasePath, identity.RootID, now)
	if err != nil {
		return cleanup(err)
	}
	owner.Store = store
	owner.Identity = identity
	if !exists {
		if !created {
			_ = store.Close()
			return cleanup(domain.NewError("UNSUPPORTED_DATABASE", "root identity is missing for an existing database", http.StatusInternalServerError))
		}
		if err = writeJSONAtomic(identityPath, identity, 0600); err != nil {
			_ = store.Close()
			return cleanup(err)
		}
	}
	_ = writeJSONAtomic(filepath.Join(control, "owner.json"), map[string]any{"mode": mode, "pid": os.Getpid(), "started_at": now.UTC()}, 0600)
	for _, dir := range []string{"log", "staging"} {
		if err = os.MkdirAll(filepath.Join(control, dir), 0700); err != nil {
			_ = owner.Close()
			return nil, err
		}
	}
	return owner, nil
}

func hasSubjectIdentity(rootPath string) bool {
	entries, err := os.ReadDir(rootPath)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == ".nornsd_root" {
			continue
		}
		if _, err = os.Stat(filepath.Join(rootPath, entry.Name(), ".nornsd", "subject.json")); err == nil {
			return true
		}
	}
	return false
}

func readIdentity(path string) (Identity, bool, error) {
	var id Identity
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return id, false, nil
	}
	if err != nil {
		return id, false, err
	}
	if err = json.Unmarshal(b, &id); err != nil {
		return id, false, domain.Wrap("UNSUPPORTED_DATABASE", "invalid root.json", http.StatusInternalServerError, err)
	}
	if id.FormatVersion != 1 || id.DatabaseSchema != version.DatabaseSchema || id.RootID == "" {
		return id, false, domain.NewError("UNSUPPORTED_DATABASE", "unsupported root identity", http.StatusInternalServerError)
	}
	return id, true, nil
}

func writeJSONAtomic(path string, value any, mode os.FileMode) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	dir := filepath.Dir(path)
	tmp := filepath.Join(dir, "."+filepath.Base(path)+"."+uuid.NewString()+".tmp")
	if err = os.WriteFile(tmp, b, mode); err != nil {
		return err
	}
	if err = os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (o *Owner) Close() error {
	var errs []error
	if o.Store != nil {
		errs = append(errs, o.Store.Close())
	}
	if o.lock != nil {
		errs = append(errs, o.lock.Unlock())
	}
	return errors.Join(errs...)
}
func (o *Owner) SubjectPath(directoryName string) (string, error) {
	clean := filepath.Clean(directoryName)
	if clean == "." || filepath.IsAbs(clean) || filepath.Base(clean) != clean {
		return "", fmt.Errorf("invalid subject directory")
	}
	path := filepath.Join(o.Path, clean)
	rel, err := filepath.Rel(o.Path, path)
	if err != nil || rel == ".." || filepath.IsAbs(rel) {
		return "", fmt.Errorf("subject directory escapes root")
	}
	return path, nil
}
