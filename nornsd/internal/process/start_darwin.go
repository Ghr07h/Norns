//go:build darwin

package process

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func unixStartIdentityNative(pid int) (string, error) {
	info, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return "", err
	}
	start := info.Proc.P_starttime
	return fmt.Sprintf("%d.%06d", start.Sec, start.Usec), nil
}
