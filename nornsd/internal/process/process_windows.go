//go:build windows

package process

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type platformState struct {
	mu    sync.Mutex
	known map[uint32]string
}

var kernel32 = windows.NewLazySystemDLL("kernel32.dll")
var psapi = windows.NewLazySystemDLL("psapi.dll")
var procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
var procGenerateConsoleCtrlEvent = kernel32.NewProc("GenerateConsoleCtrlEvent")
var procGetProcessMemoryInfo = psapi.NewProc("GetProcessMemoryInfo")

func configureCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}
func configureInteractiveCommand(cmd *exec.Cmd) { configureCommand(cmd) }
func attachPlatform(cmd *exec.Cmd) (platformState, error) {
	pid := uint32(cmd.Process.Pid)
	start, err := windowsStartIdentity(pid)
	if err != nil {
		return platformState{}, err
	}
	return platformState{known: map[uint32]string{pid: start}}, nil
}
func attachInteractivePlatform(cmd *exec.Cmd) (platformState, error) {
	return attachPlatform(cmd)
}
func closePlatform(state *platformState) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.known = nil
}

func resourceControl(state *platformState) ResourceControl {
	return ResourceControl{MemoryAccounting: "process_tree_rss", MemoryEnforced: false}
}

func processIdentity(pid int, state *platformState) (Identity, error) {
	start, err := windowsStartIdentity(uint32(pid))
	return Identity{Platform: runtime.GOOS, PID: pid, StartIdentity: start}, err
}

func recoverTree(ctx context.Context, identity Identity) (RecoveryStatus, error) {
	if identity.Platform != runtime.GOOS || identity.PID <= 0 || identity.StartIdentity == "" {
		return RecoveryUnknown, nil
	}
	current, err := processIdentity(identity.PID, nil)
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return RecoveryAbsent, nil
		}
		return RecoveryUnknown, err
	}
	if current.StartIdentity != identity.StartIdentity {
		return RecoveryUnknown, nil
	}
	state := platformState{known: map[uint32]string{uint32(identity.PID): identity.StartIdentity}}
	if err = terminateWindowsTree(ctx, uint32(identity.PID), &state); err != nil {
		return RecoveryOwned, err
	}
	for {
		_, err = windowsStartIdentity(uint32(identity.PID))
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return RecoveryOwned, nil
		}
		if err != nil {
			return RecoveryOwned, err
		}
		select {
		case <-ctx.Done():
			return RecoveryOwned, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}
func interruptProcess(cmd *exec.Cmd, state *platformState) error {
	if cmd.Process == nil {
		return nil
	}
	result, _, callErr := procGenerateConsoleCtrlEvent.Call(uintptr(1), uintptr(cmd.Process.Pid))
	if result == 0 {
		return callErr
	}
	return nil
}
func terminateTree(ctx context.Context, cmd *exec.Cmd, state *platformState) error {
	if cmd.Process == nil {
		return nil
	}
	return terminateWindowsTree(ctx, uint32(cmd.Process.Pid), state)
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

func exitSignal(err error) *string { return nil }

type processMemoryCounters struct {
	CB                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
	PrivateUsage               uintptr
}

func treeRSS(root int, state *platformState) (uint64, error) {
	ids, err := windowsOwnedProcesses(uint32(root), state)
	if err != nil {
		return 0, err
	}
	var total uint64
	for _, pid := range ids {
		handle, openErr := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ, false, pid)
		if openErr != nil {
			continue
		}
		counters := processMemoryCounters{CB: uint32(unsafe.Sizeof(processMemoryCounters{}))}
		ok, _, _ := procGetProcessMemoryInfo.Call(uintptr(handle), uintptr(unsafe.Pointer(&counters)), uintptr(counters.CB))
		_ = windows.CloseHandle(handle)
		if ok != 0 {
			total += uint64(counters.WorkingSetSize)
		}
	}
	return total, nil
}

func windowsStartIdentity(pid uint32) (string, error) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(handle)
	var created, exited, kernel, user windows.Filetime
	if err = windows.GetProcessTimes(handle, &created, &exited, &kernel, &user); err != nil {
		return "", err
	}
	return strconv.FormatInt(created.Nanoseconds(), 10), nil
}

func windowsProcessSnapshot() (map[uint32]uint32, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	if err = windows.Process32First(snapshot, &entry); err != nil {
		return nil, err
	}
	items := map[uint32]uint32{}
	for {
		items[entry.ProcessID] = entry.ParentProcessID
		err = windows.Process32Next(snapshot, &entry)
		if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
			return items, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func windowsOwnedProcesses(root uint32, state *platformState) ([]uint32, error) {
	items, err := windowsProcessSnapshot()
	if err != nil {
		return nil, err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.known == nil {
		state.known = map[uint32]string{}
	}
	for pid, expected := range state.known {
		if _, alive := items[pid]; !alive {
			delete(state.known, pid)
			continue
		}
		if expected != "" {
			actual, identityErr := windowsStartIdentity(pid)
			if identityErr != nil || actual != expected {
				delete(state.known, pid)
			}
		}
	}
	if _, ok := state.known[root]; !ok {
		if _, alive := items[root]; alive {
			if start, identityErr := windowsStartIdentity(root); identityErr == nil {
				state.known[root] = start
			}
		}
	}
	for changed := true; changed; {
		changed = false
		for pid, parent := range items {
			if _, alreadyOwned := state.known[pid]; alreadyOwned {
				continue
			}
			if _, parentOwned := state.known[parent]; !parentOwned {
				continue
			}
			start, identityErr := windowsStartIdentity(pid)
			if identityErr != nil {
				continue
			}
			state.known[pid] = start
			changed = true
		}
	}
	ids := make([]uint32, 0, len(state.known))
	for pid := range state.known {
		ids = append(ids, pid)
	}
	sort.Slice(ids, func(i, j int) bool {
		if ids[i] == root {
			return ids[j] != root
		}
		if ids[j] == root {
			return false
		}
		return ids[i] > ids[j]
	})
	return ids, nil
}

func terminateWindowsTree(ctx context.Context, root uint32, state *platformState) error {
	var firstErr error
	for attempt := 0; attempt < 3; attempt++ {
		select {
		case <-ctx.Done():
			if firstErr != nil {
				return firstErr
			}
			return ctx.Err()
		default:
		}
		ids, err := windowsOwnedProcesses(root, state)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return firstErr
		}
		for _, pid := range ids {
			if err = terminateWindowsProcess(pid); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return firstErr
}

func terminateWindowsProcess(pid uint32) error {
	handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return nil
	}
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	err = windows.TerminateProcess(handle, 1)
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return nil
	}
	return err
}

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

func physicalMemory() (uint64, error) {
	status := memoryStatusEx{Length: uint32(unsafe.Sizeof(memoryStatusEx{}))}
	ok, _, err := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&status)))
	if ok == 0 {
		return 0, err
	}
	if status.TotalPhys == 0 {
		return 0, fmt.Errorf("physical memory is unavailable")
	}
	return status.TotalPhys, nil
}
