package output

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/Ghr07h/norns/norns/internal/domain"
)

func TestSuccessEnvelopeOrderAndEmptyWarnings(t *testing.T) {
	var buffer bytes.Buffer
	if err := WriteSuccess(&buffer, map[string]any{"id": "x"}, nil, "done"); err != nil {
		t.Fatal(err)
	}
	text := buffer.String()
	assertOrder(t, text, `"ok"`, `"data"`, `"warnings"`, `"message"`)
	if !strings.Contains(text, `"warnings":[]`) || !strings.HasSuffix(text, "\n") {
		t.Fatalf("invalid success envelope: %s", text)
	}
}

func TestErrorEnvelopeExitCodesAndDetails(t *testing.T) {
	for _, test := range []struct {
		err  error
		code int
		name string
	}{
		{domain.NewError("INVALID_ARGUMENT", "bad"), 2, "INVALID_ARGUMENT"},
		{domain.NewError("NO_OPEN_TASK", "none"), 1, "NO_OPEN_TASK"},
		{errors.New("unexpected"), 1, "INTERNAL_ERROR"},
	} {
		var buffer bytes.Buffer
		if code := WriteError(&buffer, test.err); code != test.code {
			t.Fatalf("exit code=%d, want %d", code, test.code)
		}
		text := buffer.String()
		assertOrder(t, text, `"ok"`, `"error"`)
		if !strings.Contains(text, `"code":"`+test.name+`"`) || !strings.Contains(text, `"details":{}`) {
			t.Fatalf("invalid failure envelope: %s", text)
		}
	}
}

func assertOrder(t *testing.T, value string, parts ...string) {
	t.Helper()
	last := -1
	for _, part := range parts {
		index := strings.Index(value, part)
		if index <= last {
			t.Fatalf("%q is missing or out of order: %s", part, value)
		}
		last = index
	}
}
