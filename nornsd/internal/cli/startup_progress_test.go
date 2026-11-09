package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestServerStartupPhaseReportsStartAndCompletion(t *testing.T) {
	var output bytes.Buffer
	calls := 0
	err := runServerStartupPhase(&output, "recovering subjects and runtimes", time.Hour, func() error {
		calls++
		if !strings.Contains(output.String(), "nornsd: recovering subjects and runtimes...\n") {
			t.Error("startup phase was not reported before recovery began")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("work called %d times, want 1", calls)
	}
	text := output.String()
	if !strings.Contains(text, "recovering subjects and runtimes completed in ") {
		t.Fatalf("missing completion message: %s", text)
	}
	if strings.Contains(text, "still ") || strings.Contains(text, "listening on") {
		t.Fatalf("unexpected waiting or readiness message: %s", text)
	}
}

func TestServerStartupPhasePreservesFailureAndCancellation(t *testing.T) {
	for _, failure := range []error{errors.New("recovery failed"), context.Canceled} {
		t.Run(failure.Error(), func(t *testing.T) {
			var output bytes.Buffer
			err := runServerStartupPhase(&output, "recovering subjects and runtimes", time.Hour, func() error {
				return failure
			})
			if err != failure {
				t.Fatalf("error = %v, want original %v", err, failure)
			}
			text := output.String()
			if !strings.Contains(text, "recovering subjects and runtimes failed after ") {
				t.Fatalf("missing failure message: %s", text)
			}
			if strings.Contains(text, "completed") || strings.Contains(text, "listening on") {
				t.Fatalf("failure incorrectly reported success: %s", text)
			}
		})
	}
}

type startupProgressWriter chan string

func (out startupProgressWriter) Write(data []byte) (int, error) {
	out <- string(data)
	return len(data), nil
}

func TestServerStartupPhaseReportsWaitingUntilWorkFinishes(t *testing.T) {
	output := make(startupProgressWriter, 16)
	err := runServerStartupPhase(output, "recovering subjects and runtimes", time.Millisecond, func() error {
		select {
		case text := <-output:
			if !strings.Contains(text, "nornsd: recovering subjects and runtimes...\n") {
				t.Errorf("unexpected first message: %s", text)
			}
		case <-time.After(5 * time.Second):
			return errors.New("missing startup message")
		}
		select {
		case text := <-output:
			for _, want := range []string{"still recovering subjects and runtimes", "elapsed ", "HTTP API is not ready yet"} {
				if !strings.Contains(text, want) {
					t.Errorf("waiting message does not contain %q: %s", want, text)
				}
			}
		case <-time.After(5 * time.Second):
			return errors.New("missing waiting message")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var last string
	for len(output) > 0 {
		last = <-output
	}
	if !strings.Contains(last, "recovering subjects and runtimes completed in ") {
		t.Fatalf("last message did not report completion: %s", last)
	}
}

func TestServerStartupPhasePreservesPanicWithoutReportingCompletion(t *testing.T) {
	var output bytes.Buffer
	failure := errors.New("unexpected recovery panic")
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = runServerStartupPhase(&output, "recovering subjects and runtimes", time.Hour, func() error {
			panic(failure)
		})
	}()
	if recovered != failure {
		t.Fatalf("panic = %v, want original %v", recovered, failure)
	}
	if strings.Contains(output.String(), "completed") {
		t.Fatalf("panic incorrectly reported completion: %s", output.String())
	}
}
