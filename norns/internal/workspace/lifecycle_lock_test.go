package workspace

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLifecycleLockRemovesFileAfterLastHolder(t *testing.T) {
	paths, err := Resolve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, shared := range []bool{true, false} {
		if err := os.WriteFile(paths.Lock, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		lock := NewLock(paths)
		acquire := lock.Exclusive
		if shared {
			acquire = lock.Shared
		}
		release, err := acquire(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(paths.Lock); err != nil {
			t.Fatalf("active operation has no lock file: %v", err)
		}
		if shared {
			other, err := NewLock(paths).Shared(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if err := release(); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(paths.Lock); err != nil {
				t.Fatalf("lock removed while another reader was active: %v", err)
			}
			release = other
		}
		if err := release(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(paths.Lock); !os.IsNotExist(err) {
			t.Fatalf("idle workspace retained its lock file: %v", err)
		}
		if err := release(); err != nil {
			t.Fatalf("repeated release failed: %v", err)
		}
	}
}

func TestLifecycleLockRejectsDeletedFileAndPreservesReplacement(t *testing.T) {
	paths, err := Resolve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	release, err := NewLock(paths).Exclusive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	stale, err := openLifecycleFile(paths.Lock)
	if err != nil {
		t.Fatal(err)
	}
	defer stale.Close()
	if err := release(); err != nil {
		t.Fatal(err)
	}
	currentRelease, err := NewLock(paths).Exclusive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer currentRelease()
	if acquired, err := tryAcquireLifecycleFile(stale, paths.Lock, true); err != nil || acquired {
		t.Fatalf("stale waiter must retry the current path: acquired=%t err=%v", acquired, err)
	}
	if acquired, err := tryLifecycleFileLock(stale, false); err != nil || !acquired {
		t.Fatalf("cannot prepare stale holder: acquired=%t err=%v", acquired, err)
	}
	if err := releaseLifecycleFile(stale, paths.Lock, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.Lock); err != nil {
		t.Fatalf("stale cleanup removed the replacement lock: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	if _, err := NewLock(paths).Shared(ctx); errorCode(err) != "WORKSPACE_BUSY" {
		t.Fatalf("replacement lock lost exclusion: %v", err)
	}
}

func TestLifecycleLockCancellationDoesNotCreateFile(t *testing.T) {
	paths, err := Resolve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewLock(paths).Shared(ctx); errorCode(err) != "WORKSPACE_BUSY" {
		t.Fatalf("cancelled acquisition returned %v", err)
	}
	if _, err := os.Stat(paths.Lock); !os.IsNotExist(err) {
		t.Fatalf("cancelled acquisition created a lock file: %v", err)
	}
}

func TestLifecycleLockConcurrentCleanupPreservesExclusion(t *testing.T) {
	paths, err := Resolve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var readers atomic.Int64
	var writers atomic.Int64
	var workers sync.WaitGroup
	start := make(chan struct{})
	for worker := 0; worker < 8; worker++ {
		workers.Add(1)
		go func(shared bool) {
			defer workers.Done()
			<-start
			for iteration := 0; iteration < 20; iteration++ {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				acquire := NewLock(paths).Exclusive
				if shared {
					acquire = NewLock(paths).Shared
				}
				release, err := acquire(ctx)
				cancel()
				if err != nil {
					t.Errorf("acquire: %v", err)
					return
				}
				if shared {
					readers.Add(1)
					if writers.Load() != 0 {
						t.Error("reader overlapped with a writer")
					}
				} else {
					if count := writers.Add(1); count != 1 || readers.Load() != 0 {
						t.Errorf("exclusive holder overlapped: readers=%d writers=%d", readers.Load(), count)
					}
				}
				time.Sleep(100 * time.Microsecond)
				if shared {
					readers.Add(-1)
				} else {
					writers.Add(-1)
				}
				if err := release(); err != nil {
					t.Errorf("release: %v", err)
					return
				}
			}
		}(worker%3 != 0)
	}
	close(start)
	workers.Wait()
	if _, err := os.Stat(paths.Lock); !os.IsNotExist(err) {
		t.Fatalf("concurrent cleanup left a lock file: %v", err)
	}
}
