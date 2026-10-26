package extension

import "testing"

func TestExtensionNames(t *testing.T) {
	for _, name := range []string{"websearch", "git-tools", "x1"} {
		if !IsValidName(name) {
			t.Fatalf("valid extension name rejected: %s", name)
		}
	}
	for _, name := range []string{"", "Web", "1tool", "../tool", "tool_name"} {
		if IsValidName(name) {
			t.Fatalf("invalid extension name accepted: %s", name)
		}
	}
}
