//go:build linux

package process

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLinuxEffectiveMemoryUsesCgroupV2Limit(t *testing.T) {
	root := t.TempDir()
	meminfo := filepath.Join(root, "meminfo")
	selfCgroup := filepath.Join(root, "self.cgroup")
	cgroupRoot := filepath.Join(root, "cgroup")
	current := filepath.Join(cgroupRoot, "docker", "runtime")
	if err := os.MkdirAll(current, 0700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, meminfo, "MemTotal:       33554432 kB\n")
	writeTestFile(t, selfCgroup, "0::/docker/runtime\n")
	writeTestFile(t, filepath.Join(current, "memory.max"), "17179869184\n")

	memory, err := linuxEffectiveMemory(meminfo, cgroupRoot, selfCgroup)
	if err != nil {
		t.Fatal(err)
	}
	if want := uint64(16 * 1024 * 1024 * 1024); memory != want {
		t.Fatalf("effective memory=%d, want %d", memory, want)
	}
}

func TestLinuxEffectiveMemoryFallsBackWhenCgroupV2IsUnlimited(t *testing.T) {
	root := t.TempDir()
	meminfo := filepath.Join(root, "meminfo")
	selfCgroup := filepath.Join(root, "self.cgroup")
	cgroupRoot := filepath.Join(root, "cgroup")
	if err := os.MkdirAll(cgroupRoot, 0700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, meminfo, "MemTotal:       33554432 kB\n")
	writeTestFile(t, selfCgroup, "0::/\n")
	writeTestFile(t, filepath.Join(cgroupRoot, "memory.max"), "max\n")

	memory, err := linuxEffectiveMemory(meminfo, cgroupRoot, selfCgroup)
	if err != nil {
		t.Fatal(err)
	}
	if want := uint64(32 * 1024 * 1024 * 1024); memory != want {
		t.Fatalf("effective memory=%d, want %d", memory, want)
	}
}

func TestLinuxEffectiveMemoryUsesCgroupV1Limit(t *testing.T) {
	root := t.TempDir()
	meminfo := filepath.Join(root, "meminfo")
	selfCgroup := filepath.Join(root, "self.cgroup")
	cgroupRoot := filepath.Join(root, "cgroup")
	current := filepath.Join(cgroupRoot, "memory", "docker", "runtime")
	if err := os.MkdirAll(current, 0700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, meminfo, "MemTotal:       33554432 kB\n")
	writeTestFile(t, selfCgroup, "5:memory:/docker/runtime\n")
	writeTestFile(t, filepath.Join(current, "memory.limit_in_bytes"), "8589934592\n")

	memory, err := linuxEffectiveMemory(meminfo, cgroupRoot, selfCgroup)
	if err != nil {
		t.Fatal(err)
	}
	if want := uint64(8 * 1024 * 1024 * 1024); memory != want {
		t.Fatalf("effective memory=%d, want %d", memory, want)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
