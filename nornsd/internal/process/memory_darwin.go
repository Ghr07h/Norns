//go:build darwin

package process

import (
	"os/exec"
	"strconv"
	"strings"
)

func physicalMemory() (uint64, error) {
	out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
}
