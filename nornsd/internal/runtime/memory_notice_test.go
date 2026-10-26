package runtime

import (
	"strings"
	"testing"

	"github.com/Ghr07h/norns/nornsd/internal/domain"
)

func TestMemoryRecoveryNoticeUsesImmediatePreviousRuntime(t *testing.T) {
	memoryCode := memoryLimitExceededCode
	runtimes := []domain.Runtime{
		{ID: "current"},
		{ID: "previous", ErrorCode: &memoryCode},
	}
	notice, previousID := memoryRecoveryNoticeFor(runtimes, "current")
	if previousID != "previous" || !strings.Contains(notice, "内存异常增长") || !strings.Contains(notice, "继续执行当前指令") {
		t.Fatalf("unexpected recovery notice: id=%q notice=%q", previousID, notice)
	}
}

func TestMemoryRecoveryNoticeDoesNotSkipUnrelatedPreviousRuntime(t *testing.T) {
	memoryCode := memoryLimitExceededCode
	otherCode := "AGENT_EXITED"
	runtimes := []domain.Runtime{
		{ID: "current"},
		{ID: "previous", ErrorCode: &otherCode},
		{ID: "older", ErrorCode: &memoryCode},
	}
	notice, previousID := memoryRecoveryNoticeFor(runtimes, "current")
	if notice != "" || previousID != "" {
		t.Fatalf("unexpected stale recovery notice: id=%q notice=%q", previousID, notice)
	}
}

func TestMemoryRecoveryNoticeEligibility(t *testing.T) {
	tests := []struct {
		name            string
		norns           bool
		resumeSessionID string
		want            bool
	}{
		{name: "norns new session", norns: true, want: true},
		{name: "norns resumed session", norns: true, resumeSessionID: "previous", want: true},
		{name: "plain new session", want: false},
		{name: "plain resumed session", resumeSessionID: "previous", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := domain.RuntimeProfile{}
			if tt.norns {
				profile.Capabilities = []string{"norns"}
			}
			got := memoryRecoveryNoticeEnabled(profile, tt.resumeSessionID)
			if got != tt.want {
				t.Fatalf("eligibility=%t, want %t", got, tt.want)
			}
		})
	}
}

func TestMemoryLimitFailureReasonMatchesErrorCode(t *testing.T) {
	if got := runtimeFailureReason(memoryLimitExceededCode); got != memoryLimitExceededCode {
		t.Fatalf("memory failure reason=%q, want %q", got, memoryLimitExceededCode)
	}
	if got := runtimeFailureReason("AGENT_PROTOCOL_ERROR"); got != "runtime failed" {
		t.Fatalf("ordinary failure reason=%q", got)
	}
}
