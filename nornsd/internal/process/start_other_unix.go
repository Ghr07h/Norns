//go:build !windows && !linux && !darwin

package process

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func unixStartIdentityNative(pid int) (string, error) {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "lstart=").Output()
	if err != nil {
		return "", err
	}
	start := strings.Join(strings.Fields(string(out)), " ")
	if start == "" {
		return "", fmt.Errorf("process start identity is unavailable for pid %d", pid)
	}
	return start, nil
}
