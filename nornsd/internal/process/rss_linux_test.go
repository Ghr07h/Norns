//go:build linux

package process

import (
	"os"
	"testing"
)

func TestParseProcStat(t *testing.T) {
	data := []byte("123 (worker (search)) S 10 123 123 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 100 4096 256")
	stat, err := parseProcStat(data)
	if err != nil {
		t.Fatal(err)
	}
	if stat.parent != 10 || stat.group != 123 || stat.rss != 256*uint64(os.Getpagesize()) {
		t.Fatalf("unexpected stat: %#v", stat)
	}
}
