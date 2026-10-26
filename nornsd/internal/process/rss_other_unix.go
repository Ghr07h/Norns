//go:build !windows && !linux

package process

import (
	"bufio"
	"os/exec"
	"strconv"
	"strings"
)

func processTreeRSS(root int) (uint64, error) {
	cmd := exec.Command("ps", "-axo", "pid=,ppid=,pgid=,rss=")
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	type item struct {
		parent int
		group  int
		rss    uint64
	}
	items := map[int]item{}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 4 {
			continue
		}
		pid, _ := strconv.Atoi(fields[0])
		ppid, _ := strconv.Atoi(fields[1])
		pgid, _ := strconv.Atoi(fields[2])
		rss, _ := strconv.ParseUint(fields[3], 10, 64)
		items[pid] = item{parent: ppid, group: pgid, rss: rss * 1024}
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
	return total, scanner.Err()
}
