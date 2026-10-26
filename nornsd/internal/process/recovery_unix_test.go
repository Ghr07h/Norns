//go:build !windows

package process

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestStartInteractiveKeepsForegroundProcessGroup(t *testing.T) {
	managed, err := StartInteractive(exec.Command("sleep", "60"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_ = managed.TerminateTree(ctx)
		cancel()
		_, _ = managed.Wait()
	}()
	identity, err := managed.Identity()
	if err != nil {
		t.Fatal(err)
	}
	if identity.PGID != syscall.Getpgrp() {
		t.Fatalf("interactive PGID=%d, parent PGID=%d", identity.PGID, syscall.Getpgrp())
	}
}

func TestRecoverTreeRequiresMatchingStartIdentity(t *testing.T) {
	managed, err := Start(exec.Command("sh", "-c", "sleep 60"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_ = managed.TerminateTree(ctx)
		cancel()
		_, _ = managed.Wait()
	}()
	identity, err := managed.Identity()
	if err != nil {
		t.Fatal(err)
	}
	if identity.PGID != identity.PID || identity.PGID == syscall.Getpgrp() {
		t.Fatalf("managed process did not get an independent process group: %#v", identity)
	}
	mismatch := identity
	mismatch.StartIdentity += "-different"
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	status, err := RecoverTree(ctx, mismatch)
	cancel()
	if err != nil || status != RecoveryUnknown {
		t.Fatalf("mismatched identity status=%s err=%v", status, err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	status, err = RecoverTree(ctx, identity)
	cancel()
	if err != nil || status != RecoveryOwned {
		t.Fatalf("matching identity status=%s err=%v", status, err)
	}
}

func TestWaitKillsResidualProcessGroup(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "child.pid")
	managed, err := Start(exec.Command("sh", "-c", `sleep 60 & echo $! > "$1"`, "sh", pidPath))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err = managed.WaitContext(ctx); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		err = syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("residual process %d is still alive", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
