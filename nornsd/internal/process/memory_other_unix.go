//go:build !windows && !linux && !darwin

package process

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

func physicalMemory() (uint64, error) {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "MemTotal:" {
			kb, parseErr := strconv.ParseUint(fields[1], 10, 64)
			return kb * 1024, parseErr
		}
	}
	return 0, errors.New("physical memory is unavailable")
}
