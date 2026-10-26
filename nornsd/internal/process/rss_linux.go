//go:build linux

package process

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type procStat struct {
	parent int
	group  int
	rss    uint64
}

func processTreeRSS(root int) (uint64, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, err
	}
	items := make(map[int]procStat, len(entries))
	for _, entry := range entries {
		pid, parseErr := strconv.Atoi(entry.Name())
		if parseErr != nil {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join("/proc", entry.Name(), "stat"))
		if readErr != nil {
			continue // Processes may exit while /proc is being scanned.
		}
		stat, parseErr := parseProcStat(data)
		if parseErr == nil {
			items[pid] = stat
		}
	}

	owned := map[int]bool{root: true}
	for pid, item := range items {
		if item.group == root {
			owned[pid] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for pid, item := range items {
			if !owned[pid] && owned[item.parent] {
				owned[pid] = true
				changed = true
			}
		}
	}

	var total uint64
	for pid := range owned {
		total += items[pid].rss
	}
	return total, nil
}

func parseProcStat(data []byte) (procStat, error) {
	// comm is parenthesized and may itself contain spaces or parentheses.
	end := strings.LastIndexByte(string(data), ')')
	if end < 0 || end+1 >= len(data) {
		return procStat{}, fmt.Errorf("invalid /proc stat record")
	}
	fields := strings.Fields(string(data[end+1:]))
	// fields starts at proc(5) field 3 (state); RSS is field 24.
	if len(fields) <= 21 {
		return procStat{}, fmt.Errorf("incomplete /proc stat record")
	}
	parent, err := strconv.Atoi(fields[1])
	if err != nil {
		return procStat{}, err
	}
	group, err := strconv.Atoi(fields[2])
	if err != nil {
		return procStat{}, err
	}
	rssPages, err := strconv.ParseInt(fields[21], 10, 64)
	if err != nil {
		return procStat{}, err
	}
	if rssPages < 0 {
		rssPages = 0
	}
	return procStat{parent: parent, group: group, rss: uint64(rssPages) * uint64(os.Getpagesize())}, nil
}
