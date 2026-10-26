//go:build linux

package process

import (
	"fmt"
	"os"
	"strings"
)

func unixStartIdentityNative(pid int) (string, error) {
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return "", err
	}
	end := strings.LastIndexByte(string(stat), ')')
	if end < 0 {
		return "", fmt.Errorf("invalid /proc stat for pid %d", pid)
	}
	fields := strings.Fields(string(stat[end+1:]))
	if len(fields) <= 19 {
		return "", fmt.Errorf("incomplete /proc stat for pid %d", pid)
	}
	bootID, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(bootID)) + ":" + fields[19], nil
}
