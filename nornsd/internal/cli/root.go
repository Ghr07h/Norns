package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	ccagent "github.com/Ghr07h/norns/nornsd/internal/agent/cc"
	"github.com/Ghr07h/norns/nornsd/internal/app"
	"github.com/Ghr07h/norns/nornsd/internal/config"
	"github.com/Ghr07h/norns/nornsd/internal/domain"
	"github.com/Ghr07h/norns/nornsd/internal/eventlog"
	"github.com/Ghr07h/norns/nornsd/internal/httpapi"
	nornsadapter "github.com/Ghr07h/norns/nornsd/internal/norns"
	"github.com/Ghr07h/norns/nornsd/internal/plugin"
	"github.com/Ghr07h/norns/nornsd/internal/process"
	"github.com/Ghr07h/norns/nornsd/internal/root"
	runtimemanager "github.com/Ghr07h/norns/nornsd/internal/runtime"
	"github.com/Ghr07h/norns/nornsd/internal/runtimeartifact"
	"github.com/Ghr07h/norns/nornsd/internal/version"
)

func Execute(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	rootCmd := newRoot(stdin, stdout, stderr)
	rootCmd.SetArgs(args)
	rootCmd.SetIn(stdin)
	rootCmd.SetOut(stdout)
	rootCmd.SetErr(stderr)
	if err := rootCmd.Execute(); err != nil {
		if errors.Is(err, errLaunchCancelled) {
			return 130
		}
		return writeError(stderr, err)
	}
	return 0
}

func newRoot(stdin io.Reader, stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{Use: "nornsd", Short: "Subject-oriented Agent runtime daemon", SilenceUsage: true, SilenceErrors: true}
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return domain.NewError("INVALID_REQUEST", err.Error(), http.StatusBadRequest)
	})
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.AddCommand(agentCommand(stdin, stdout, stderr), serverCommand(stdout, stderr), versionCommand(stdout))
	return cmd
}

func versionCommand(out io.Writer) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{Use: "version", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
		value := map[string]any{"version": version.ProductVersion, "api_schema": version.APISchema, "database_schema": version.DatabaseSchema, "agent_config_version": version.AgentConfigVersion, "norns_protocol": version.NornsProtocol, "plugins": map[string]int{"cc": version.CCPluginVersion}, "claude_code_version": version.ClaudeCodeVersion, "commit": version.Commit, "build_time": version.BuildTime}
		if asJSON {
			return json.NewEncoder(out).Encode(value)
		}
		_, err := fmt.Fprintf(out, "nornsd %s (API schema %d, Norns protocol %d, Claude Code %s)\n", version.ProductVersion, version.APISchema, version.NornsProtocol, version.ClaudeCodeVersion)
		return err
	}}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON")
	return cmd
}

type serverOptions struct {
	host  string
	port  int
	token string
}

func serverCommand(stdout, stderr io.Writer) *cobra.Command {
	var files []string
	cmd := &cobra.Command{Use: "server", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if len(files) == 0 {
			return domain.NewError("INVALID_SETTING", "--configs is required", http.StatusBadRequest)
		}
		templates := make([]config.Config, len(files))
		for i, file := range files {
			cfg, err := config.LoadPartial(file)
			if err != nil {
				return domain.Wrap("INVALID_SETTING", "cannot load nornsd config", http.StatusBadRequest, err)
			}
			if i > 0 {
				cfg.UseService(templates[0])
			}
			cfg, err = config.Normalize(cfg)
			if err != nil {
				return domain.Wrap("INVALID_SETTING", "invalid nornsd config", http.StatusBadRequest, err)
			}
			templates[i] = cfg
		}
		first := templates[0]
		if first.ServiceRoot == "" || !filepath.IsAbs(first.ServiceRoot) {
			return domain.NewError("INVALID_SETTING", "SERVICE_NORNSD_ROOT_DIR must be an absolute path", http.StatusBadRequest)
		}
		if strings.TrimSpace(first.NornsServerToken) == "" {
			return domain.NewError("INVALID_SETTING", "SERVICE_TOKEN is required", http.StatusBadRequest)
		}
		concurrency := first.ServiceConcurrency
		if len(templates) != 1 && len(templates) != concurrency {
			return domain.NewError("INVALID_SETTING", "--configs count must be one or equal to SERVICE_CONCURRENCY", http.StatusBadRequest)
		}
		options := serverOptions{host: first.NornsServerHost, port: first.NornsServerPort, token: first.NornsServerToken}
		var owner *root.Owner
		err := runServerStartupPhase(stderr, "opening workspace and checking database", serverStartupProgressInterval, func() error {
			var acquireErr error
			owner, acquireErr = root.Acquire(cmd.Context(), first.ServiceRoot, "server", time.Now().UTC())
			return acquireErr
		})
		if err != nil {
			return err
		}
		defer owner.Close()
		var norns *nornsadapter.Adapter
		if adapter, discoverErr := nornsadapter.Discover(""); discoverErr == nil {
			norns = &adapter
		}
		loaded := make([]*config.Config, concurrency+1)
		for slot := 1; slot <= concurrency; slot++ {
			index := 0
			if len(templates) > 1 {
				index = slot - 1
			}
			loaded[slot] = &templates[index]
			cfg := loaded[slot]
			probeCtx, cancel := context.WithTimeout(cmd.Context(), version.ControlTimeout)
			err = ccagent.Preflight(probeCtx, cfg.AgentExec, ccagent.RequiresExactVersion(cfg.AgentExec, cfg.AgentExecExplicit()))
			cancel()
			if err != nil {
				return domain.Wrap("AGENT_UNAVAILABLE", "Claude Code preflight failed", http.StatusServiceUnavailable, err)
			}
			if cfg.NornsEnable {
				if norns == nil {
					return domain.NewError("NORNS_UNAVAILABLE", "norns CLI is unavailable", http.StatusServiceUnavailable)
				}
				probeCtx, cancel := context.WithTimeout(cmd.Context(), version.ControlTimeout)
				err = norns.Preflight(probeCtx)
				cancel()
				if err != nil {
					return err
				}
			}
		}
		memory, err := process.PhysicalMemory()
		if err != nil {
			return err
		}
		logger := slog.New(slog.NewJSONHandler(stderr, nil))
		manager := runtimemanager.NewManager(cmd.Context(), runtimemanager.Options{Owner: owner, Concurrency: concurrency, SlotConfigs: loaded, Norns: norns, MemoryLimit: memory / 2, OnError: func(err error) { logger.Error("runtime manager", "error", err) }, OnQueueExport: func(path string, count int) { logger.Info("queued runtimes exported", "path", path, "count", count) }})
		// Export the old queue with the previous template copies before replacing them.
		if err = runServerStartupPhase(stderr, "exporting queued runtimes and recovering subjects", serverStartupProgressInterval, func() error { return manager.Recover(cmd.Context()) }); err != nil {
			return err
		}
		if err = config.SaveTemplates(owner.Control, templates); err != nil {
			return err
		}
		listener, err := net.Listen("tcp", net.JoinHostPort(options.host, strconv.Itoa(options.port)))
		if err != nil {
			return err
		}
		defer listener.Close()
		manager.Start()
		defer manager.Close()
		service := &app.Service{Owner: owner, Store: owner.Store, Manager: manager, Norns: norns}
		api := httpapi.New(httpapi.Config{Service: service, Token: options.token, Concurrency: concurrency, Logger: logger})
		server := &http.Server{Handler: api, ReadHeaderTimeout: version.RequestTimeout}

		if !isLoopbackHost(options.host) {
			_, _ = fmt.Fprintln(stderr, "warning: nornsd is serving a Bearer token over plaintext HTTP on a non-loopback address; use a trusted network or TLS reverse proxy")
		}
		_, _ = fmt.Fprintf(stdout, "nornsd listening on http://%s\n", listener.Addr())
		serveErr := make(chan error, 1)
		go func() {
			err := server.Serve(listener)
			if !errors.Is(err, http.ErrServerClosed) {
				serveErr <- err
			} else {
				serveErr <- nil
			}
		}()
		signals := make(chan os.Signal, 2)
		signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(signals)
		select {
		case err = <-serveErr:
			return err
		case <-signals:
		}
		api.SetDraining()
		manager.Drain(false)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), version.RequestTimeout)
		_ = server.Shutdown(shutdownCtx)
		cancel()
		waitDone := make(chan struct{})
		go func() { manager.Wait(); close(waitDone) }()
		select {
		case <-waitDone:
		case <-signals:
			manager.Drain(true)
			<-waitDone
		}
		exportCtx, exportCancel := context.WithTimeout(context.Background(), version.GracefulStopTimeout)
		defer exportCancel()
		return manager.ExportQueued(exportCtx, "server shutdown")
	}}
	cmd.Flags().StringArrayVar(&files, "configs", nil, "nornsd.yaml Runtime templates (repeat for each concurrency slot)")
	return cmd
}

const interactiveSubjectName = "交互式对话 Subject"

var errLaunchCancelled = errors.New("interactive Runtime launch cancelled")

func newInteractiveSubject(now time.Time, useNorns bool) domain.Subject {
	id := uuid.NewString()
	name := interactiveSubjectName
	return domain.Subject{
		ID:             id,
		Name:           &name,
		DirectoryName:  root.DirectoryName(now, id),
		WorkspaceState: domain.WorkspaceMaterializing,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func agentCommand(stdin io.Reader, stdout, stderr io.Writer) *cobra.Command {
	var resumeSession bool
	var resumeSubject, setting string
	cmd := &cobra.Command{Use: "agent -- [AGENT_ARGS...]", Args: cobra.ArbitraryArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 && cmd.ArgsLenAtDash() < 0 {
			return domain.NewError("INVALID_REQUEST", "Agent arguments must follow --", http.StatusBadRequest)
		}
		if resumeSession && resumeSubject == "" {
			return domain.NewError("INVALID_REQUEST", "--resume-session requires --resume-subject", http.StatusBadRequest)
		}
		if setting == "" {
			return domain.NewError("INVALID_SETTING", "--config is required", http.StatusBadRequest)
		}
		loaded, err := config.Load(setting)
		if err != nil {
			return domain.Wrap("INVALID_SETTING", "invalid nornsd config", http.StatusBadRequest, err)
		}
		cfg := &loaded
		useNorns := cfg.NornsEnable
		if cfg.ServiceRoot == "" {
			return domain.NewError("INVALID_SETTING", "SERVICE_NORNSD_ROOT_DIR is required", http.StatusBadRequest)
		}
		owner, err := root.Acquire(cmd.Context(), cfg.ServiceRoot, "agent", time.Now().UTC())
		if err != nil {
			return err
		}
		defer owner.Close()
		recovery := runtimemanager.NewManager(cmd.Context(), runtimemanager.Options{Owner: owner, Concurrency: 1, OnQueueExport: func(path string, count int) {
			_, _ = fmt.Fprintf(stderr, "nornsd: exported %d queued runtimes to %s\n", count, path)
		}})
		if err = recovery.Recover(cmd.Context()); err != nil {
			return err
		}
		if err = config.SaveTemplates(owner.Control, []config.Config{loaded}); err != nil {
			return err
		}
		var subject domain.Subject
		resuming := resumeSubject != ""
		if resuming {
			if _, err = uuid.Parse(resumeSubject); err != nil {
				return domain.NewError("INVALID_REQUEST", "--resume-subject must be a Subject UUID", http.StatusBadRequest)
			}
			subject, err = owner.Store.GetSubject(cmd.Context(), resumeSubject)
			if err != nil {
				return err
			}
			if subject.State != domain.SubjectIdle {
				return domain.NewError("SUBJECT_RUNTIME_ACTIVE", "subject already has an active runtime", http.StatusConflict)
			}
		}
		execArgs := []string{"claude"}
		strict := true
		if cfg != nil {
			execArgs = cfg.AgentExec
			strict = ccagent.RequiresExactVersion(cfg.AgentExec, cfg.AgentExecExplicit())
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), version.ControlTimeout)
		err = ccagent.Preflight(ctx, execArgs, strict)
		cancel()
		if err != nil {
			return domain.Wrap("AGENT_UNAVAILABLE", "Claude Code preflight failed", http.StatusServiceUnavailable, err)
		}
		var norns *nornsadapter.Adapter
		hadOpenNornsTask := false
		if useNorns {
			adapter, discoverErr := nornsadapter.Discover("")
			if discoverErr != nil {
				return domain.Wrap("NORNS_UNAVAILABLE", "norns CLI is unavailable", http.StatusServiceUnavailable, discoverErr)
			}
			ctx, cancel = context.WithTimeout(cmd.Context(), version.ControlTimeout)
			err = adapter.Preflight(ctx)
			cancel()
			if err != nil {
				return domain.Wrap("NORNS_UNAVAILABLE", "norns CLI preflight failed", http.StatusServiceUnavailable, err)
			}
			norns = &adapter
		}
		if !resuming {
			now := time.Now().UTC()
			subject = newInteractiveSubject(now, useNorns)
			if err = owner.Store.CreateSubject(cmd.Context(), subject, nil, "", now); err != nil {
				return err
			}
			subject, err = owner.Store.GetSubject(cmd.Context(), subject.ID)
			if err != nil {
				return err
			}
			if _, err = owner.MaterializeSubject(cmd.Context(), subject); err != nil {
				return err
			}
		}
		path, err := owner.SubjectPath(subject.DirectoryName)
		if err != nil {
			return err
		}
		runtimeID := uuid.NewString()
		runtime, resumeSessionID, err := owner.Store.CreateDirectRuntimeWithSession(cmd.Context(), subject.ID, runtimeID, resumeSession, time.Now().UTC())
		if err != nil {
			return err
		}
		runtime.Profile = cfg.Profile()
		if err = owner.Store.SaveRuntimeConfig(cmd.Context(), runtime.ID, runtime.Profile, cfg); err != nil {
			return err
		}
		defer func() {
			current, getErr := owner.Store.GetRuntime(context.Background(), subject.ID, runtimeID, false)
			if getErr == nil && !current.State.Terminal() {
				_ = owner.Store.SetRuntimeTerminalWithDetails(context.Background(), subject.ID, runtimeID, domain.RuntimeError, "runtime preparation failed", "RUNTIME_PREPARATION_FAILED", nil, time.Now().UTC(), true)
			}
		}()

		settingsPath, configHash, err := config.Publish(cfg.AgentType, path, runtimeID, cfg)
		if err != nil {
			return err
		}
		preparedPlugins, err := plugin.Prepare(cfg.AgentType, path, runtimeID, cfg)
		if err != nil {
			return err
		}
		pluginPaths, rootAgent := preparedPlugins.Paths, preparedPlugins.RootAgent

		launch := ccagent.Launch{Exec: execArgs, Passthrough: args, SessionID: runtimeID, ResumeSessionID: resumeSessionID, SettingsPath: settingsPath, PluginPaths: pluginPaths, RootAgent: rootAgent, Dir: path}
		createdAt := time.Now().UTC()
		snapshot, publishErr := runtimeartifact.Publish(path, runtimeartifact.Input{Subject: subject, Runtime: runtime, Launch: launch, Managed: false, Config: cfg, ConfigHash: configHash, CreatedAt: createdAt})
		if publishErr != nil {
			_ = owner.Store.SetRuntimeTerminalWithDetails(context.Background(), subject.ID, runtimeID, domain.RuntimeError, "runtime snapshot failed", "RUNTIME_SNAPSHOT_FAILED", map[string]any{"message": publishErr.Error()}, time.Now().UTC(), true)
			return publishErr
		}
		if err = owner.Store.SaveConfigSnapshot(cmd.Context(), runtimeID, snapshot, createdAt); err != nil {
			_ = owner.Store.SetRuntimeTerminalWithDetails(context.Background(), subject.ID, runtimeID, domain.RuntimeError, "runtime snapshot failed", "RUNTIME_SNAPSHOT_FAILED", map[string]any{"message": err.Error()}, time.Now().UTC(), true)
			return err
		}
		logger := eventlog.New(owner.Store, path, subject.ID, runtimeID)
		if err = eventlog.SyncReadable(cmd.Context(), owner.Store, path, subject.ID); err != nil {
			_ = owner.Store.SetRuntimeTerminalWithDetails(context.Background(), subject.ID, runtimeID, domain.RuntimeError, "event projector failed", "EVENT_PROJECTOR_FAILED", map[string]any{"message": err.Error()}, time.Now().UTC(), true)
			return err
		}
		if _, err = logger.Record(cmd.Context(), "runtime", "lifecycle", json.RawMessage(`{"phase":"launching","mode":"interactive"}`), time.Now().UTC()); err != nil {
			_ = owner.Store.SetRuntimeTerminalWithDetails(context.Background(), subject.ID, runtimeID, domain.RuntimeError, "event projector failed", "EVENT_PROJECTOR_FAILED", map[string]any{"message": err.Error()}, time.Now().UTC(), true)
			return err
		}
		_, _ = logger.Event(cmd.Context(), "runtime.started", "runtime", map[string]any{"mode": "interactive"}, time.Now().UTC())
		if norns != nil {
			ctx, cancel := context.WithTimeout(cmd.Context(), version.ControlTimeout)
			hadOpenNornsTask, _ = norns.HasOpenTask(ctx, path)
			cancel()
		}
		forwarded := make(chan os.Signal, 2)
		signal.Notify(forwarded, os.Interrupt, syscall.SIGTERM)
		var userInterrupted atomic.Bool
		signalDone := make(chan struct{})
		signalStarted := false
		var directIdentity process.Identity
		result, agentErr := ccagent.RunInteractiveWithStart(cmd.Context(), launch, stdin, stdout, stderr, func(p *process.Managed) error {
			identity, identityErr := p.Identity()
			if identityErr != nil {
				return identityErr
			}
			directIdentity = identity
			processInfo := map[string]any{"identity": identity, "session_id": runtimeID, "mode": "interactive", "strict_version": strict}
			if resumeSessionID != "" {
				processInfo["resume_session_id"] = resumeSessionID
			}
			if err := owner.Store.UpdateRuntimeLaunch(context.Background(), subject.ID, runtimeID, configHash, processInfo, time.Now().UTC()); err != nil {
				return err
			}
			signalStarted = true
			go func() {
				defer close(signalDone)
				count := 0
				for range forwarded {
					userInterrupted.Store(true)
					count++
					signalPayload, _ := json.Marshal(map[string]any{"phase": "signal", "count": count})
					_, _ = logger.Record(context.Background(), "runtime", "lifecycle", signalPayload, time.Now().UTC())
					if count == 1 {
						_ = p.Interrupt()
					} else {
						ctx, cancel := context.WithTimeout(context.Background(), version.ControlTimeout)
						_ = p.TerminateTree(ctx)
						cancel()
					}
				}
			}()
			return nil
		})
		signal.Stop(forwarded)
		close(forwarded)
		if signalStarted {
			<-signalDone
		}
		exitPayload, _ := json.Marshal(map[string]any{"phase": "exited", "exit_code": result.ExitCode, "wait_error": agentErr != nil})
		var finalErr error = agentErr
		if _, recordErr := logger.Record(context.Background(), "runtime", "lifecycle", exitPayload, time.Now().UTC()); recordErr != nil && finalErr == nil {
			finalErr = recordErr
		}
		processInfo := map[string]any{"identity": directIdentity, "session_id": runtimeID, "result": result}
		if resumeSessionID != "" {
			processInfo["resume_session_id"] = resumeSessionID
		}
		_ = owner.Store.UpdateRuntimeProcess(context.Background(), subject.ID, runtimeID, processInfo)
		transcript, archiveErr := ccagent.ArchiveTranscript(path, runtimeID, runtimeID)
		if archiveErr != nil {
			transcript.Status = "failed"
			logger.Diagnostic(context.Background(), "TRANSCRIPT_ARCHIVE_FAILED", archiveErr)
		}
		_ = owner.Store.UpdateRuntimeTranscript(context.Background(), subject.ID, runtimeID, transcript)
		state, reason, code := domain.RuntimeCancelled, "user_exit", ""
		agentFailed := agentErr != nil || result.ExitCode != 0
		if agentFailed && !userInterrupted.Load() {
			state, reason, code = domain.RuntimeError, "agent exited with an error", "AGENT_EXITED"
		}
		if norns != nil && state == domain.RuntimeCancelled && hadOpenNornsTask {
			ctx, cancel := context.WithTimeout(context.Background(), version.ControlTimeout)
			open, _ := norns.HasOpenTask(ctx, path)
			cancel()
			if !open {
				state, reason = domain.RuntimeEnded, "norns task closed"
			}
		}
		var errorDetails any
		if code != "" {
			details := map[string]any{"message": reason, "exit_code": result.ExitCode, "signal": result.Signal}
			if agentErr != nil {
				details["cause"] = agentErr.Error()
			}
			errorDetails = details
		}
		_ = owner.Store.SetRuntimeTerminalWithDetails(context.Background(), subject.ID, runtimeID, state, reason, code, errorDetails, time.Now().UTC(), false)
		_, _ = logger.Event(context.Background(), "runtime.finished", "runtime", eventlog.RuntimeFinishedPayload(state, result.ExitCode, reason, code), time.Now().UTC())
		_ = owner.Store.FinalizeRuntime(context.Background(), subject.ID, runtimeID, time.Now().UTC())
		if norns != nil {
			_, _ = fmt.Fprintln(stderr, "Runtime 配置与 transcript 已保存在 Root 的 .nornsd/subjects/ 中。")
		}
		if agentFailed && !userInterrupted.Load() {
			if agentErr == nil {
				agentErr = fmt.Errorf("Claude Code exited with status %d", result.ExitCode)
			}
			return domain.Wrap("AGENT_EXITED", "Claude Code exited with an error", http.StatusBadGateway, agentErr)
		}
		return finalErr
	}}
	cmd.Flags().BoolVar(&resumeSession, "resume-session", false, "fork the latest Runtime session into the new Runtime (requires --resume-subject)")
	cmd.Flags().StringVar(&resumeSubject, "resume-subject", "", "reuse a Subject UUID and create a new Runtime")
	cmd.Flags().StringVar(&setting, "config", "", "required nornsd.yaml configuration")
	return cmd
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
func writeError(w io.Writer, err error) int {
	de := domain.AsError(err)
	details := make(map[string]any, len(de.Details)+1)
	for key, value := range de.Details {
		details[key] = value
	}
	if de.Cause != nil {
		details["cause"] = de.Cause.Error()
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"code": de.Code, "message": de.Message, "details": details}})
	if de.HTTPStatus == http.StatusBadRequest {
		return 2
	}
	return 1
}
