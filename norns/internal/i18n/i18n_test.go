package i18n

import "testing"

func TestMessageUsesLocalePriority(t *testing.T) {
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("LC_MESSAGES", "zh_CN.UTF-8")
	t.Setenv("LC_ALL", "")
	if got := Message("Task opened"); got != "Task 已打开" {
		t.Fatalf("LC_MESSAGES was not selected: %q", got)
	}
	t.Setenv("LC_ALL", "C.UTF-8")
	if got := Message("Task opened"); got != "Task opened" {
		t.Fatalf("LC_ALL did not override LC_MESSAGES: %q", got)
	}
}

func TestMessageLocalizesParameterizedText(t *testing.T) {
	t.Setenv("LC_ALL", "zh-CN")
	if got := Message("invalid task ID: bad"); got != "无效的 task ID: bad" {
		t.Fatalf("unexpected ID message: %q", got)
	}
	if got := Message("purpose must not be empty"); got != "purpose 不能为空" {
		t.Fatalf("unexpected empty-value message: %q", got)
	}
}

func TestErrorFallsBackWithoutChangingCodeContext(t *testing.T) {
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	got := Error("RESET_FAILED", "cannot sync history directory")
	if got != "Workspace reset 失败: cannot sync history directory" {
		t.Fatalf("unexpected fallback: %q", got)
	}
}
