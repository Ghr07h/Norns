//go:build linux

package process

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	linuxMeminfoPath = "/proc/meminfo"
	linuxSelfCgroup  = "/proc/self/cgroup"
	linuxCgroupRoot  = "/sys/fs/cgroup"
)

func physicalMemory() (uint64, error) {
	return linuxEffectiveMemory(linuxMeminfoPath, linuxCgroupRoot, linuxSelfCgroup)
}

func linuxEffectiveMemory(meminfoPath, cgroupRoot, selfCgroupPath string) (uint64, error) {
	physical, err := readLinuxPhysicalMemory(meminfoPath)
	if err != nil {
		return 0, err
	}
	if limit, ok := readLinuxCgroupMemoryLimit(cgroupRoot, selfCgroupPath); ok && limit < physical {
		return limit, nil
	}
	return physical, nil
}

func readLinuxPhysicalMemory(path string) (uint64, error) {
	b, err := os.ReadFile(path)
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

func readLinuxCgroupMemoryLimit(root, selfCgroupPath string) (uint64, bool) {
	b, err := os.ReadFile(selfCgroupPath)
	if err != nil {
		return 0, false
	}
	var unifiedPath, legacyPath string
	for _, line := range strings.Split(string(b), "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			continue
		}
		if parts[0] == "0" && parts[1] == "" {
			unifiedPath = parts[2]
		}
		for _, controller := range strings.Split(parts[1], ",") {
			if controller == "memory" {
				legacyPath = parts[2]
				break
			}
		}
	}

	var limit uint64
	found := false
	consider := func(value uint64, ok bool) {
		if ok && value > 0 && (!found || value < limit) {
			limit, found = value, true
		}
	}
	if unifiedPath != "" {
		for _, dir := range cgroupSearchDirs(root, unifiedPath) {
			consider(readMemoryLimitFile(filepath.Join(dir, "memory.max")))
		}
	}
	if legacyPath != "" {
		for _, base := range []string{filepath.Join(root, "memory"), root} {
			for _, dir := range cgroupSearchDirs(base, legacyPath) {
				consider(readMemoryLimitFile(filepath.Join(dir, "memory.limit_in_bytes")))
			}
		}
	}
	return limit, found
}

func cgroupSearchDirs(root, relative string) []string {
	root = filepath.Clean(root)
	relative = strings.TrimPrefix(filepath.Clean("/"+relative), string(filepath.Separator))
	dir := filepath.Join(root, relative)
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}
	var dirs []string
	for {
		dirs = append(dirs, dir)
		if dir == root {
			return dirs
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return dirs
		}
		dir = parent
	}
}

func readMemoryLimitFile(path string) (uint64, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	value := strings.TrimSpace(string(b))
	if value == "" || value == "max" {
		return 0, false
	}
	limit, err := strconv.ParseUint(value, 10, 64)
	return limit, err == nil
}
