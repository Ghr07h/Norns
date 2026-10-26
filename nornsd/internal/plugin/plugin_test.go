package plugin

import "testing"

func TestPublishRejectsUnsupportedAgent(t *testing.T) {
	if _, err := Publish("unknown", t.TempDir(), "runtime-1", nil); err == nil {
		t.Fatal("expected unsupported Agent Plugin generation to fail")
	}
}
