//go:build !windows

package process

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"time"
)

func configureCommand(cmd *exec.Cmd)            { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
func configureInteractiveCommand(cmd *exec.Cmd) {}

type platformState struct {
	interactive bool
}

func attachPlatform(cmd *exec.Cmd) (platformState, error) {
	return platformState{}, nil
}
func attachInteractivePlatform(cmd *exec.Cmd) (platformState, error) {
	return platformState{interactive: true}, nil
}
func closePlatform(state *platformState) {}

func resourceControl(state *platformState) ResourceControl {
	return ResourceControl{MemoryAccounting: "process_tree_rss", MemoryEnforced: false}
}

func processIdentity(pid int, state *platformState) (Identity, error) {
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		return Identity{}, err
	}
	start, err := unixStartIdentityNative(pid)
	if err != nil {
		return Identity{}, err
	}
	return Identity{Platform: runtime.GOOS, PID: pid, PGID: pgid, StartIdentity: start}, nil
}

func recoverTree(ctx context.Context, identity Identity) (RecoveryStatus, error) {
	if identity.Platform != runtime.GOOS || identity.PID <= 0 || identity.PGID <= 0 || identity.StartIdentity == "" {
		return RecoveryUnknown, nil
	}
	current, err := processIdentity(identity.PID, nil)
	if err != nil {
		if processAbsent(identity.PID) {
			return RecoveryAbsent, nil
		}
		return RecoveryUnknown, err
	}
	if current.PGID != identity.PGID || current.StartIdentity != identity.StartIdentity {
		return RecoveryUnknown, nil
	}
	if err = syscall.Kill(-identity.PGID, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return RecoveryOwned, err
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if processAbsent(identity.PID) {
			return RecoveryOwned, nil
		}
		current, currentErr := processIdentity(identity.PID, nil)
		if currentErr == nil && current.StartIdentity != identity.StartIdentity {
			return RecoveryOwned, nil
		}
		select {
		case <-ctx.Done():
			return RecoveryOwned, ctx.Err()
		case <-ticker.C:
		}
	}
}

func processAbsent(pid int) bool {
	err := syscall.Kill(pid, 0)
	return errors.Is(err, syscall.ESRCH)
}
func interruptProcess(cmd *exec.Cmd, state *platformState) error {
	if cmd.Process == nil {
		return nil
	}
	if state != nil && state.interactive {
		return cmd.Process.Signal(os.Interrupt)
	}
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
}
func terminateTree(ctx context.Context, cmd *exec.Cmd, state *platformState) error {
	if cmd.Process == nil {
		return nil
	}
	if state != nil && state.interactive {
		err := cmd.Process.Kill()
		if errors.Is(err, os.ErrProcessDone) {
			return nil
		}
		return err
	}
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH) {
		return nil
	}
	if err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	return -1
}

func exitSignal(err error) *string {
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		return nil
	}
	status, ok := exit.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() {
		return nil
	}
	value := status.Signal().String()
	return &value
}

func treeRSS(root int, state *platformState) (uint64, error) {
	return processTreeRSS(root)
}
