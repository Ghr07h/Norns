package cc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/Ghr07h/norns/nornsd/internal/process"
	"github.com/Ghr07h/norns/nornsd/internal/version"
)

var versionPattern = regexp.MustCompile(`\b([0-9]+\.[0-9]+\.[0-9]+)\b`)

const sandboxEnvironment = "IS_SANDBOX=1"

type Launch struct {
	Exec            []string
	Passthrough     []string
	SessionID       string
	ResumeSessionID string
	SettingsPath    string
	PluginPaths     []string
	RootAgent       string
	Dir             string
	Env             []string
}

func Preflight(ctx context.Context, argv []string, strict bool) error {
	if len(argv) == 0 {
		return fmt.Errorf("empty AGENT_EXEC")
	}
	path, err := exec.LookPath(argv[0])
	if err != nil {
		return fmt.Errorf("agent executable not found: %w; install with npm install -g @anthropic-ai/claude-code@%s", err, version.ClaudeCodeVersion)
	}
	if !strict {
		return nil
	}
	args := append(append([]string{}, argv[1:]...), "--version")
	cmd := exec.CommandContext(ctx, path, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Claude Code version probe failed: %w", err)
	}
	match := versionPattern.FindStringSubmatch(string(out))
	if len(match) < 2 || match[1] != version.ClaudeCodeVersion {
		return fmt.Errorf("Claude Code %s is required; install with npm install -g @anthropic-ai/claude-code@%s", version.ClaudeCodeVersion, version.ClaudeCodeVersion)
	}
	return nil
}

func RequiresExactVersion(argv []string, explicit bool) bool {
	if !explicit {
		return true
	}
	if len(argv) == 0 {
		return false
	}
	name := strings.ToLower(path.Base(strings.ReplaceAll(argv[0], `\`, "/")))
	switch name {
	case "claude", "claude.exe", "claude.cmd":
		return true
	default:
		return false
	}
}

func BuildArgs(l Launch, managed bool) ([]string, error) {
	if len(l.Exec) == 0 {
		return nil, fmt.Errorf("empty agent command")
	}
	if l.SessionID == "" {
		return nil, fmt.Errorf("empty agent session ID")
	}
	if l.ResumeSessionID == l.SessionID {
		return nil, fmt.Errorf("resume session ID must differ from the new session ID")
	}
	reserved := map[string]bool{"--session-id": true, "--resume": true, "--fork-session": true, "--input-format": true, "--output-format": true, "--include-partial-messages": true, "--dangerously-skip-permissions": true, "--setting-sources": true}
	for _, arg := range l.Passthrough {
		key := arg
		if i := strings.IndexByte(key, '='); i >= 0 {
			key = key[:i]
		}
		if reserved[key] || (l.SettingsPath != "" && key == "--settings") || (len(l.PluginPaths) > 0 && key == "--plugin-dir") || (l.RootAgent != "" && key == "--agent") {
			return nil, fmt.Errorf("agent argument %s conflicts with nornsd control", key)
		}
	}
	argv := append([]string{}, l.Exec...)
	argv = append(argv, l.Passthrough...)
	argv = append(argv, "--dangerously-skip-permissions")
	if managed {
		argv = append(argv, "--output-format", "stream-json", "--verbose", "--input-format", "stream-json", "--include-partial-messages")
	}
	if l.ResumeSessionID != "" {
		argv = append(argv, "--resume="+l.ResumeSessionID, "--fork-session")
	}
	argv = append(argv, "--session-id="+l.SessionID)
	if l.SettingsPath != "" {
		argv = append(argv, "--settings", l.SettingsPath)
	}
	if managed || l.SettingsPath != "" {
		argv = append(argv, "--setting-sources=")
	}
	for _, pluginPath := range l.PluginPaths {
		argv = append(argv, "--plugin-dir", pluginPath)
	}
	if l.RootAgent != "" {
		argv = append(argv, "--agent", l.RootAgent)
	}
	return argv, nil
}

type RawHandler func(stream, kind string, payload json.RawMessage) error
type MessageHandler func(json.RawMessage) error

type Session struct {
	launch    Launch
	cmd       *exec.Cmd
	proc      *process.Managed
	stdin     io.WriteCloser
	out       io.ReadCloser
	errOut    io.ReadCloser
	raw       RawHandler
	onMessage MessageHandler
	writeMu   sync.Mutex
	mu        sync.Mutex
	waiters   map[string]chan controlResult
	done      chan struct{}
	readErr   error
}
type controlResult struct {
	Success bool
	Error   string
}

func StartManaged(ctx context.Context, l Launch, raw RawHandler, onMessage MessageHandler) (*Session, error) {
	argv, err := BuildArgs(l, true)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = l.Dir
	cmd.Env = launchEnvironment(l.Env)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	p, err := process.Start(cmd)
	if err != nil {
		return nil, err
	}
	s := &Session{launch: l, cmd: cmd, proc: p, stdin: stdin, out: stdout, errOut: stderr, raw: raw, onMessage: onMessage, waiters: map[string]chan controlResult{}, done: make(chan struct{})}
	go s.readStdout()
	go s.readStderr()
	return s, nil
}

func RunInteractive(ctx context.Context, l Launch, stdin io.Reader, stdout, stderr io.Writer) (process.Result, error) {
	return RunInteractiveWithStart(ctx, l, stdin, stdout, stderr, nil)
}
func RunInteractiveWithStart(ctx context.Context, l Launch, stdin io.Reader, stdout, stderr io.Writer, onStart func(*process.Managed) error) (process.Result, error) {
	argv, err := BuildArgs(l, false)
	if err != nil {
		return process.Result{}, err
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = l.Dir
	cmd.Env = launchEnvironment(l.Env)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	p, err := process.StartInteractive(cmd)
	if err != nil {
		return process.Result{}, err
	}
	if onStart != nil {
		if err = onStart(p); err != nil {
			stopCtx, cancel := context.WithTimeout(context.Background(), version.ControlTimeout)
			_ = p.TerminateTree(stopCtx)
			cancel()
			_, _ = p.Wait()
			return process.Result{}, err
		}
	}
	return p.Wait()
}

func launchEnvironment(extra []string) []string {
	env := append(os.Environ(), extra...)
	return append(env, sandboxEnvironment)
}

func (s *Session) Initialize(ctx context.Context) error {
	return s.control(ctx, "initialize", map[string]any{"hooks": nil, "forwardSubagentText": true})
}
func (s *Session) Interrupt(ctx context.Context) error { return s.control(ctx, "interrupt", nil) }
func (s *Session) control(ctx context.Context, subtype string, extra map[string]any) error {
	id := "req_" + uuid.NewString()
	request := map[string]any{"subtype": subtype}
	for k, v := range extra {
		request[k] = v
	}
	ch := make(chan controlResult, 1)
	s.mu.Lock()
	s.waiters[id] = ch
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.waiters, id); s.mu.Unlock() }()
	if err := s.write(map[string]any{"type": "control_request", "request_id": id, "request": request}); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case result := <-ch:
		if !result.Success {
			return fmt.Errorf("control %s failed: %s", subtype, result.Error)
		}
		return nil
	case <-s.done:
		return fmt.Errorf("agent exited during %s", subtype)
	}
}

func (s *Session) SendUser(content string) error {
	return s.write(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": content}, "parent_tool_use_id": nil, "session_id": s.launch.SessionID})
}
func (s *Session) write(value any) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if s.raw != nil {
		if err = s.raw("stdin", "agent_message", append(json.RawMessage(nil), b...)); err != nil {
			return err
		}
	}
	b = append(b, '\n')
	_, err = s.stdin.Write(b)
	return err
}

func (s *Session) readStdout() {
	defer close(s.done)
	reader := bufio.NewReaderSize(s.out, 64*1024)
	for {
		line, err := readFrame(reader, version.MaxAgentFrameBytes)
		if len(line) > 0 {
			trimmed := bytes.TrimSpace(line)
			if !json.Valid(trimmed) {
				s.failRead(fmt.Errorf("invalid agent JSON"))
				return
			}
			raw := append(json.RawMessage(nil), trimmed...)
			if s.onMessage != nil {
				if messageErr := s.onMessage(raw); messageErr != nil {
					s.failRead(messageErr)
					return
				}
			} else if s.raw != nil {
				if rawErr := s.raw("stdout", "agent_message", raw); rawErr != nil {
					s.failRead(rawErr)
					return
				}
			}
			s.handleControl(raw)
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				s.setReadErr(err)
			}
			return
		}
	}
}
func (s *Session) readStderr() {
	reader := bufio.NewReaderSize(s.errOut, 32*1024)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 && s.raw != nil {
			payload, _ := json.Marshal(map[string]any{"text": string(line)})
			if rawErr := s.raw("stderr", "stderr", payload); rawErr != nil {
				s.failRead(rawErr)
				return
			}
		}
		if err != nil {
			return
		}
	}
}
func readFrame(r *bufio.Reader, max int) ([]byte, error) {
	var out []byte
	for {
		part, prefix, err := r.ReadLine()
		if len(out)+len(part) > max {
			return nil, fmt.Errorf("agent frame exceeds %d bytes", max)
		}
		out = append(out, part...)
		if err != nil {
			return out, err
		}
		if !prefix {
			return out, nil
		}
	}
}
func (s *Session) handleControl(raw json.RawMessage) {
	var envelope struct {
		Type     string `json:"type"`
		Response struct {
			RequestID string `json:"request_id"`
			Subtype   string `json:"subtype"`
			Error     string `json:"error"`
		} `json:"response"`
	}
	if json.Unmarshal(raw, &envelope) != nil || envelope.Type != "control_response" || envelope.Response.RequestID == "" {
		return
	}
	success := envelope.Response.Subtype == "success"
	s.mu.Lock()
	ch := s.waiters[envelope.Response.RequestID]
	s.mu.Unlock()
	if ch != nil {
		select {
		case ch <- controlResult{Success: success, Error: envelope.Response.Error}:
		default:
		}
	}
}
func (s *Session) CloseInput() error { return s.stdin.Close() }
func (s *Session) Wait() (process.Result, error) {
	result, err := s.proc.Wait()
	s.mu.Lock()
	readErr := s.readErr
	s.mu.Unlock()
	if readErr != nil {
		return result, fmt.Errorf("agent protocol: %w", readErr)
	}
	return result, err
}
func (s *Session) Terminate(ctx context.Context) error { return s.proc.TerminateTree(ctx) }
func (s *Session) RSS() (uint64, error)                { return s.proc.TreeRSS() }
func (s *Session) PID() int                            { return s.proc.PID() }
func (s *Session) Identity() (process.Identity, error) { return s.proc.Identity() }
func (s *Session) ResourceControl() process.ResourceControl {
	return s.proc.ResourceControl()
}

func (s *Session) setReadErr(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	if s.readErr == nil {
		s.readErr = err
	}
	s.mu.Unlock()
}

func (s *Session) failRead(err error) {
	s.setReadErr(err)
	ctx, cancel := context.WithTimeout(context.Background(), version.ControlTimeout)
	defer cancel()
	_ = s.proc.TerminateTree(ctx)
}
