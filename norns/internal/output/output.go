package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/Ghr07h/norns/norns/internal/domain"
	"github.com/Ghr07h/norns/norns/internal/i18n"
)

type Warning struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

type Success struct {
	OK       bool      `json:"ok"`
	Data     any       `json:"data"`
	Warnings []Warning `json:"warnings"`
	Message  string    `json:"message"`
}

type failureBody struct {
	OK    bool    `json:"ok"`
	Error failure `json:"error"`
}
type failure struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

func JSON(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(value)
}

func WriteSuccess(w io.Writer, data any, warnings []Warning, message string) error {
	if warnings == nil {
		warnings = []Warning{}
	}
	localized := make([]Warning, len(warnings))
	copy(localized, warnings)
	for index := range localized {
		localized[index].Message = i18n.Message(localized[index].Message)
	}
	return JSON(w, Success{OK: true, Data: data, Warnings: localized, Message: i18n.Message(message)})
}

func WriteError(w io.Writer, err error) int {
	var de *domain.Error
	if !errors.As(err, &de) {
		de = domain.Wrap("INTERNAL_ERROR", "internal error", err)
	}
	if de.Details == nil {
		de.Details = map[string]any{}
	}
	_ = JSON(w, failureBody{OK: false, Error: failure{Code: de.Code, Message: i18n.Error(de.Code, de.Message), Details: de.Details}})
	if de.Code == "INVALID_ARGUMENT" {
		return 2
	}
	return 1
}

func ConcurrencyWarning(running, maximum int) Warning {
	return Warning{Code: "CONCURRENCY_LIMIT_EXCEEDED", Message: "operation applied, but running explorations exceed the configured concurrency", Details: map[string]any{"operation_applied": true, "running_count": running, "max_concurrency": maximum}}
}

func OpenWarning(err error) Warning {
	return Warning{Code: "VIEW_OPEN_FAILED", Message: "view generated, but the browser could not be opened", Details: map[string]any{"error": fmt.Sprint(err)}}
}
