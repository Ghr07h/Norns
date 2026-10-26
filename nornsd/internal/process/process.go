package process

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
)

type Result struct {
	ExitCode int     `json:"exit_code"`
	Signal   *string `json:"signal"`
}

type Identity struct {
	Platform      string `json:"platform"`
	PID           int    `json:"pid"`
	PGID          int    `json:"pgid,omitempty"`
	StartIdentity string `json:"start_identity"`
}

type ResourceControl struct {
	MemoryAccounting string `json:"memory_accounting"`
	MemoryEnforced   bool   `json:"memory_enforced"`
}

type RecoveryStatus string

const (
	RecoveryAbsent  RecoveryStatus = "absent"
	RecoveryOwned   RecoveryStatus = "owned"
	RecoveryUnknown RecoveryStatus = "unknown"
)

type Managed struct {
	cmd      *exec.Cmd
	waitOnce sync.Once
	waitDone chan struct{}
	waitErr  error
	result   Result
	platform platformState
}

func Start(cmd *exec.Cmd) (*Managed, error) {
	return start(cmd, false)
}

// StartInteractive keeps terminal job control with the direct Agent process.
func StartInteractive(cmd *exec.Cmd) (*Managed, error) {
	return start(cmd, true)
}

func start(cmd *exec.Cmd, interactive bool) (*Managed, error) {
	if interactive {
		configureInteractiveCommand(cmd)
	} else {
		configureCommand(cmd)
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	var state platformState
	var err error
	if interactive {
		state, err = attachInteractivePlatform(cmd)
	} else {
		state, err = attachPlatform(cmd)
	}
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, err
	}
	p := &Managed{cmd: cmd, waitDone: make(chan struct{}), platform: state}
	go p.wait()
	return p, nil
}
func (p *Managed) wait() {
	p.waitOnce.Do(func() {
		p.waitErr = p.cmd.Wait()
		p.result.ExitCode = exitCode(p.waitErr)
		p.result.Signal = exitSignal(p.waitErr)
		// The managed agent may exit before its shell/tool descendants. Always
		// clear the remaining Runtime process group before releasing the slot.
		_ = terminateTree(context.Background(), p.cmd, &p.platform)
		closePlatform(&p.platform)
		close(p.waitDone)
	})
}
func (p *Managed) PID() int {
	if p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}
func (p *Managed) Identity() (Identity, error) {
	if p.cmd.Process == nil {
		return Identity{}, fmt.Errorf("process has not started")
	}
	return processIdentity(p.cmd.Process.Pid, &p.platform)
}
func (p *Managed) Wait() (Result, error) { <-p.waitDone; return p.result, p.waitErr }
func (p *Managed) WaitContext(ctx context.Context) (Result, error) {
	select {
	case <-ctx.Done():
		return Result{}, ctx.Err()
	case <-p.waitDone:
		return p.result, p.waitErr
	}
}
func (p *Managed) Interrupt() error { return interruptProcess(p.cmd, &p.platform) }
func (p *Managed) TerminateTree(ctx context.Context) error {
	return terminateTree(ctx, p.cmd, &p.platform)
}
func (p *Managed) TreeRSS() (uint64, error)         { return treeRSS(p.PID(), &p.platform) }
func (p *Managed) ResourceControl() ResourceControl { return resourceControl(&p.platform) }
func PhysicalMemory() (uint64, error)               { return physicalMemory() }
func RecoverTree(ctx context.Context, identity Identity) (RecoveryStatus, error) {
	return recoverTree(ctx, identity)
}
