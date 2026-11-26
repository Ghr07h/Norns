package norns

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Ghr07h/norns/nornsd/internal/domain"
	"github.com/Ghr07h/norns/nornsd/internal/version"
)

type Adapter struct{ Executable string }

type ViewFormat string

const (
	YAML     ViewFormat = "yaml"
	JSON     ViewFormat = "json"
	Markdown ViewFormat = "md"
)

func ParseViewFormat(value string) (ViewFormat, error) {
	switch ViewFormat(value) {
	case "", YAML:
		return YAML, nil
	case JSON, Markdown:
		return ViewFormat(value), nil
	default:
		return "", domain.NewError("INVALID_REQUEST", "format must be yaml, json or md", http.StatusBadRequest)
	}
}

func (f ViewFormat) ContentType() string {
	switch f {
	case JSON:
		return "application/json; charset=utf-8"
	case Markdown:
		return "text/markdown; charset=utf-8"
	default:
		return "application/yaml; charset=utf-8"
	}
}

type commandError struct {
	ExitCode int
	Stderr   []byte
}

func (e *commandError) Error() string { return fmt.Sprintf("norns exited with status %d", e.ExitCode) }

func translateCommandError(err *commandError) error {
	var failure struct {
		Error struct {
			Code    string         `json:"code"`
			Message string         `json:"message"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if json.Unmarshal(bytes.TrimSpace(err.Stderr), &failure) != nil || failure.Error.Code == "" || failure.Error.Message == "" {
		return domain.Wrap("NORNS_COMMAND_FAILED", "Norns command failed", http.StatusInternalServerError, err)
	}
	status := http.StatusInternalServerError
	switch failure.Error.Code {
	case "INVALID_ARGUMENT":
		status = http.StatusBadRequest
	case "TASK_NOT_FOUND", "EXPLORATION_NOT_FOUND", "COGNITION_NOT_FOUND":
		status = http.StatusNotFound
	case "WORKSPACE_NOT_INITIALIZED", "WORKSPACE_BUSY", "INVALID_STATE", "NO_OPEN_TASK", "NO_TASK":
		status = http.StatusConflict
	}
	translated := domain.NewError(failure.Error.Code, failure.Error.Message, status)
	if failure.Error.Details != nil {
		translated.Details = failure.Error.Details
	}
	return translated
}

func Discover(executable string) (Adapter, error) {
	if executable == "" {
		executable = "norns"
	}
	path, err := exec.LookPath(executable)
	if err != nil {
		return Adapter{}, fmt.Errorf("norns executable not found: %w", err)
	}
	return Adapter{Executable: path}, nil
}

func (a Adapter) Preflight(ctx context.Context) error {
	out, _, err := a.run(ctx, "", "version", "--json")
	if err != nil {
		return err
	}
	var value struct {
		ProtocolVersion int `json:"protocol_version"`
	}
	if err = json.Unmarshal(out, &value); err != nil {
		return fmt.Errorf("invalid norns version output: %w", err)
	}
	if value.ProtocolVersion != version.NornsProtocol {
		return fmt.Errorf("norns protocol %d is required, found %d", version.NornsProtocol, value.ProtocolVersion)
	}
	return nil
}

func (a Adapter) run(ctx context.Context, dir string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, a.Executable, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		if ctx.Err() != nil {
			return stdout.Bytes(), stderr.Bytes(), ctx.Err()
		}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			failure := &commandError{exit.ExitCode(), append([]byte(nil), stderr.Bytes()...)}
			return stdout.Bytes(), stderr.Bytes(), translateCommandError(failure)
		}
		return stdout.Bytes(), stderr.Bytes(), err
	}
	return stdout.Bytes(), stderr.Bytes(), nil
}

func (a Adapter) Status(ctx context.Context, subjectPath string, taskID *string, format ViewFormat) ([]byte, string, error) {
	args := []string{"view", "status"}
	if taskID != nil {
		args = append(args, "--task-id", *taskID)
	}
	return a.generatedView(ctx, subjectPath, "status", args, format)
}
func (a Adapter) Trace(ctx context.Context, subjectPath, explorationID string, format ViewFormat) ([]byte, string, error) {
	args := []string{"view", "trace", "--exploration", explorationID}
	return a.generatedView(ctx, subjectPath, "trace", args, format)
}

func (a Adapter) generatedView(ctx context.Context, subjectPath, kind string, args []string, format ViewFormat) ([]byte, string, error) {
	format, err := ParseViewFormat(string(format))
	if err != nil {
		return nil, "", err
	}
	switch format {
	case JSON:
		args = append(args, "--json")
	case Markdown:
		args = append(args, "--md")
	}
	args = append(args, "--output-reference")
	out, _, err := a.run(ctx, subjectPath, args...)
	if err != nil {
		return nil, "", err
	}
	return readPublishedView(subjectPath, out, kind, format)
}

type publishedView struct {
	OK   *bool `json:"ok"`
	Data struct {
		Path   string `json:"path"`
		Format string `json:"format"`
	} `json:"data"`
}

func readPublishedView(subjectPath string, out []byte, kind string, format ViewFormat) ([]byte, string, error) {
	trimmed := bytes.TrimSpace(out)
	if !json.Valid(trimmed) {
		return nil, "", fmt.Errorf("norns %s returned invalid JSON", kind)
	}
	var envelope publishedView
	if err := json.Unmarshal(trimmed, &envelope); err != nil {
		return nil, "", fmt.Errorf("norns %s returned invalid JSON: %w", kind, err)
	}
	if envelope.OK == nil || !*envelope.OK || envelope.Data.Path == "" || envelope.Data.Format != string(format) {
		return nil, "", fmt.Errorf("invalid norns %s response", kind)
	}
	path, err := resolveViewPath(subjectPath, envelope.Data.Path)
	if err != nil {
		return nil, "", err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	if format == JSON && !json.Valid(b) {
		return nil, "", fmt.Errorf("norns %s file contains invalid JSON", kind)
	}
	if format == YAML {
		var value map[string]any
		if err := yaml.Unmarshal(b, &value); err != nil || value == nil {
			return nil, "", fmt.Errorf("norns %s file contains invalid YAML", kind)
		}
	}
	return b, filepath.Base(path), nil
}

func (a Adapter) Cognition(ctx context.Context, subjectPath, id string) (json.RawMessage, error) {
	return a.entity(ctx, subjectPath, "cognition", "--cognition", id)
}

func (a Adapter) Exploration(ctx context.Context, subjectPath, id string) (json.RawMessage, error) {
	return a.entity(ctx, subjectPath, "exploration", "--exploration", id)
}

func (a Adapter) entity(ctx context.Context, subjectPath, kind, flag, id string) (json.RawMessage, error) {
	out, _, err := a.run(ctx, subjectPath, kind, "get", flag, id)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		OK   bool            `json:"ok"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(out, &envelope); err != nil || !envelope.OK || len(envelope.Data) == 0 || bytes.TrimSpace(envelope.Data)[0] != '{' {
		return nil, fmt.Errorf("invalid norns %s response", kind)
	}
	return envelope.Data, nil
}

func visualArgs(lang string) ([]string, error) {
	args := []string{"view", "visual"}
	if lang == "" {
		return args, nil
	}
	if lang != "zh" && lang != "en" {
		return nil, fmt.Errorf("unsupported visual language %q", lang)
	}
	return append(args, "--lang", lang), nil
}
func (a Adapter) Visual(ctx context.Context, subjectPath, lang string) (string, error) {
	args, err := visualArgs(lang)
	if err != nil {
		return "", err
	}
	out, _, err := a.run(ctx, subjectPath, args...)
	if err != nil {
		return "", err
	}
	var envelope publishedView
	if err = json.Unmarshal(out, &envelope); err != nil || envelope.OK == nil || !*envelope.OK || envelope.Data.Path == "" {
		return "", fmt.Errorf("invalid norns visual response")
	}
	return resolveViewPath(subjectPath, envelope.Data.Path)
}

func resolveViewPath(subjectPath, publishedPath string) (string, error) {
	path := publishedPath
	if !filepath.IsAbs(path) {
		path = filepath.Join(subjectPath, path)
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	viewRoot, err := filepath.Abs(filepath.Join(subjectPath, ".norns", "view"))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(viewRoot, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("norns view path escapes workspace view directory")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", fmt.Errorf("norns view path is not a regular file")
	}
	realSubject, err := filepath.EvalSymlinks(subjectPath)
	if err != nil {
		return "", err
	}
	realViewRoot, err := filepath.EvalSymlinks(viewRoot)
	if err != nil {
		return "", err
	}
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	for _, bounds := range [][2]string{{realSubject, realViewRoot}, {realViewRoot, realPath}} {
		rel, err := filepath.Rel(bounds[0], bounds[1])
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("norns view path escapes workspace through a symbolic link")
		}
	}
	return path, nil
}

func (a Adapter) Cleanup(ctx context.Context, subjectPath, outcome, reason string) (bool, error) {
	_, stderr, err := a.run(ctx, subjectPath, "task", "close", "--outcome", outcome, "--outcome-reason", reason)
	if err == nil {
		return true, nil
	}
	var failure struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(bytes.TrimSpace(stderr), &failure) == nil && failure.Error.Code == "NO_OPEN_TASK" {
		return false, nil
	}
	return false, err
}

func (a Adapter) HasOpenTask(ctx context.Context, subjectPath string) (bool, error) {
	status, _, err := a.Status(ctx, subjectPath, nil, JSON)
	if err != nil {
		return false, err
	}
	var top map[string]json.RawMessage
	if err = json.Unmarshal(status, &top); err != nil {
		return false, err
	}
	_, ok := top["task"]
	return ok, nil
}
