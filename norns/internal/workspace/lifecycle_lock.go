package workspace

import (
	"context"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/Ghr07h/norns/norns/internal/domain"
)

type LifecycleLock struct {
	path string
}

func NewLock(paths Paths) *LifecycleLock { return &LifecycleLock{path: paths.Lock} }

func (l *LifecycleLock) Shared(ctx context.Context) (func() error, error) {
	return l.acquire(ctx, true)
}

func (l *LifecycleLock) Exclusive(ctx context.Context) (func() error, error) {
	return l.acquire(ctx, false)
}

func (l *LifecycleLock) acquire(ctx context.Context, shared bool) (func() error, error) {
	message := "cannot acquire Workspace lifecycle write lock"
	if shared {
		message = "cannot acquire Workspace lifecycle read lock"
	}
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return nil, busyWithCause(message, "workspace_lifecycle", err)
		}
		file, err := openLifecycleFile(l.path)
		if err != nil {
			if !retryLifecycleFileError(err) {
				return nil, busyWithCause(message, "workspace_lifecycle", err)
			}
		} else {
			acquired, err := tryAcquireLifecycleFile(file, l.path, shared)
			if acquired {
				var once sync.Once
				var releaseErr error
				return func() error {
					once.Do(func() { releaseErr = releaseLifecycleFile(file, l.path, shared) })
					return releaseErr
				}, nil
			}
			err = errors.Join(err, file.Close())
			if err != nil {
				return nil, busyWithCause(message, "workspace_lifecycle", err)
			}
		}
		select {
		case <-ctx.Done():
			return nil, busyWithCause(message, "workspace_lifecycle", ctx.Err())
		case <-ticker.C:
		}
	}
}

func tryAcquireLifecycleFile(file *os.File, path string, shared bool) (bool, error) {
	acquired, err := tryLifecycleFileLock(file, shared)
	if !acquired || err != nil {
		return false, err
	}
	current, err := currentLifecycleFile(file, path)
	if current && err == nil {
		return true, nil
	}
	return false, errors.Join(err, unlockLifecycleFile(file))
}

func currentLifecycleFile(file *os.File, path string) (bool, error) {
	held, err := file.Stat()
	if err != nil {
		return false, err
	}
	current, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) || retryLifecycleFileError(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return os.SameFile(held, current), nil
}

func releaseLifecycleFile(file *os.File, path string, shared bool) (err error) {
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, domain.Wrap("IO_ERROR", "cannot close Workspace lifecycle lock", closeErr))
		}
	}()
	if shared {
		if err := unlockLifecycleFile(file); err != nil {
			return domain.Wrap("IO_ERROR", "cannot release Workspace lifecycle read lock", err)
		}
		acquired, err := tryLifecycleFileLock(file, false)
		if err != nil {
			return domain.Wrap("IO_ERROR", "cannot acquire Workspace lifecycle cleanup lock", err)
		}
		if !acquired {
			return nil
		}
	}
	defer func() {
		if unlockErr := unlockLifecycleFile(file); unlockErr != nil {
			err = errors.Join(err, domain.Wrap("IO_ERROR", "cannot release Workspace lifecycle lock", unlockErr))
		}
	}()
	current, err := currentLifecycleFile(file, path)
	if err != nil {
		return domain.Wrap("IO_ERROR", "cannot inspect Workspace lifecycle lock", err)
	}
	if current {
		if err := removeLifecycleFile(file, path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return domain.Wrap("IO_ERROR", "cannot remove Workspace lifecycle lock", err)
		}
	}
	return nil
}
