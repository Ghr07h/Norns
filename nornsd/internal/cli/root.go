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
		value := map[string]any{"version": version.ProductVersion, "api_version": version.APIVersion, "database_schema": version.DatabaseSchema, "agent_config_version": version.AgentConfigVersion, "norns_protocol": version.NornsProtocol, "claude_code_version": version.ClaudeCodeVersion, "commit": version.Commit, "build_time": version.BuildTime}
		if asJSON {
			return json.NewEncoder(out).Encode(value)
		}
		_, err := fmt.Fprintf(out, "nornsd %s (API %s, Norns protocol %d, Claude Code %s)\n", version.ProductVersion, version.APIVersion, version.NornsProtocol, version.ClaudeCodeVersion)
		return err
	}}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON")
	return cmd
}

type serverOptions struct {
	host     string
	port     int
	token    string
	useNorns bool
}

func (options serverOptions) withConfig(cfg config.Config) serverOptions {
	if cfg.Has("NORNS_SERVER_HOST") {
		options.host = cfg.NornsServerHost
	}
	if cfg.Has("NORNS_SERVER_PORT") {
		options.port = cfg.NornsServerPort
	}
	if cfg.Has("NORNS_SERVER_TOKEN") {
		options.token = cfg.NornsServerToken
	}
	if cfg.Has("NORNS_ENABLE") {
		options.useNorns = cfg.NornsEnable
	}
	return options
}

func serverCommand(stdout, stderr io.Writer) *cobra.Command {
	var agent, host, token, workspace string
	var port, concurrency int
	var useNorns bool
	var settings []string
	cmd := &cobra.Command{Use: "server", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if agent != "cc" {
			return domain.NewError("AGENT_NOT_IMPLEMENTED", "only the cc agent is implemented", http.StatusBadRequest)
		}
		if concurrency < 1 {
			return domain.NewError("INVALID_REQUEST", "--concurrency must be positive", http.StatusBadRequest)
		}
		if len(settings) != 0 && len(settings) != 1 && len(settings) != concurrency {
			return domain.NewError("INVALID_SETTING", "--settings count must be 0, 1, or equal to concurrency", http.StatusBadRequest)
		}
		options := serverOptions{host: host, port: port, token: token, useNorns: useNorns}
		loaded := make([]*config.Config, concurrency+1)
		unique := map[string]struct {
			argv   []string
			strict bool
		}{}
		for slot := 1; slot <= concurrency; slot++ {
			if len(settings) == 0 {
				unique["claude\x00true"] = struct {
					argv   []string
					strict bool
				}{[]string{"claude"}, true}
				continue
			}
			index := 0
			if len(settings) == concurrency {
				index = slot - 1
			}
			cfg, loadErr := config.Load(settings[index])
			if loadErr != nil {
				return domain.Wrap("INVALID_SETTING", "cannot load agent setting", http.StatusBadRequest, loadErr)
			}
			loaded[slot] = &cfg
			if slot == 1 {
				options = options.withConfig(cfg)
			}
			strict := ccagent.RequiresExactVersion(cfg.AgentExec, cfg.AgentExecExplicit())
			key := strings.Join(cfg.AgentExec, "\x00") + strconv.FormatBool(strict)
			unique[key] = struct {
				argv   []string
				strict bool
			}{cfg.AgentExec, strict}
		}
		if strings.TrimSpace(options.host) == "" {
			return domain.NewError("INVALID_REQUEST", "server host must not be empty", http.StatusBadRequest)
		}
		if options.port < 1 || options.port > 65535 {
			return domain.NewError("INVALID_REQUEST", "server port must be between 1 and 65535", http.StatusBadRequest)
		}
		if strings.TrimSpace(options.token) == "" || strings.ContainsAny(options.token, " \t\r\n") {
			return domain.NewError("INVALID_REQUEST", "server token is required and must not contain whitespace", http.StatusBadRequest)
		}
		owner, err := root.Acquire(cmd.Context(), workspace, "server", time.Now().UTC())
		if err != nil {
			return err
		}
		defer owner.Close()
		for _, probe := range unique {
			ctx, cancel := context.WithTimeout(cmd.Context(), version.ControlTimeout)
			probeErr := ccagent.Preflight(ctx, probe.argv, probe.strict)
			cancel()
			if probeErr != nil {
				return domain.Wrap("AGENT_UNAVAILABLE", "Claude Code preflight failed", http.StatusServiceUnavailable, probeErr)
			}
		}
		var norns *nornsadapter.Adapter
		if options.useNorns {
			adapter, discoverErr := nornsadapter.Discover("")
			if discoverErr != nil {
				return domain.Wrap("NORNS_UNAVAILABLE", "norns CLI is unavailable", http.StatusServiceUnavailable, discoverErr)
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), version.ControlTimeout)
			preflightErr := adapter.Preflight(ctx)
			cancel()
			if preflightErr != nil {
				return domain.Wrap("NORNS_UNAVAILABLE", "norns CLI preflight failed", http.StatusServiceUnavailable, preflightErr)
			}
			norns = &adapter
		}
		address := net.JoinHostPort(options.host, strconv.Itoa(options.port))
		listener, err := net.Listen("tcp", address)
		if err != nil {
			return domain.Wrap("SERVICE_UNAVAILABLE", "cannot bind server listener", http.StatusServiceUnavailable, err)
		}
		defer listener.Close()
		memory, err := process.PhysicalMemory()
		if err != nil {
			return domain.Wrap("RESOURCE_MONITOR_FAILED", "cannot determine physical memory", http.StatusInternalServerError, err)
		}
		profile := runtimeProfile(options.useNorns)
		logger := slog.New(slog.NewJSONHandler(stderr, nil))
		manager := runtimemanager.NewManager(cmd.Context(), runtimemanager.Options{Owner: owner, Profile: profile, Concurrency: concurrency, SlotConfigs: loaded, Norns: norns, MemoryLimit: memory / 2, OnError: func(err error) { logger.Error("runtime manager", "error", err) }})
		if err = manager.Recover(cmd.Context()); err != nil {
			return err
		}
		manager.Start()
		defer manager.Close()
		service := &app.Service{Owner: owner, Store: owner.Store, Manager: manager, Profile: profile, Norns: norns}
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
		return nil
	}}
	cmd.Flags().StringVar(&agent, "agent", "cc", "Agent type")
	cmd.Flags().StringVar(&host, "host", "127.0.0.1", "listen host")
	cmd.Flags().IntVar(&port, "port", 28081, "listen port")
	cmd.Flags().StringVar(&token, "token", "", "Bearer token")
	cmd.Flags().StringVar(&workspace, "workspace-root", ".", "nornsd root")
	cmd.Flags().BoolVar(&useNorns, "norns", false, "enable Norns")
	cmd.Flags().StringArrayVar(&settings, "settings", nil, "agent.yaml for a concurrency slot")
	cmd.Flags().IntVar(&concurrency, "concurrency", 1, "stable runtime slots")
	return cmd
}

const bareSetting = "__nornsd_bare_setting__"
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
		Profile:        runtimeProfile(useNorns),
		SessionID:      id,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func agentCommand(stdin io.Reader, stdout, stderr io.Writer) *cobra.Command {
	var cc, cx, pi, useNorns, resumeSession bool
	var resumeSubject, setting string
	cmd := &cobra.Command{Use: "agent -- [AGENT_ARGS...]", Args: cobra.ArbitraryArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 && cmd.ArgsLenAtDash() < 0 {
			return domain.NewError("INVALID_REQUEST", "Agent arguments must follow --", http.StatusBadRequest)
		}
		selected := 0
		for _, v := range []bool{cc, cx, pi} {
			if v {
				selected++
			}
		}
		if selected > 1 {
			return domain.NewError("INVALID_REQUEST", "--cc, --cx and --pi are mutually exclusive", http.StatusBadRequest)
		}
		if resumeSession && resumeSubject == "" {
			return domain.NewError("INVALID_REQUEST", "--resume-session requires --resume-subject", http.StatusBadRequest)
		}
		if cx || pi {
			return domain.NewError("AGENT_NOT_IMPLEMENTED", "selected agent is not implemented", http.StatusBadRequest)
		}
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		owner, err := root.Acquire(cmd.Context(), cwd, "agent", time.Now().UTC())
		if err != nil {
			return err
		}
		defer owner.Close()
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
		var cfg *config.Config
		var settingSources []string
		if cmd.Flags().Changed("setting") {
			loaded, sources, loadErr := loadAgentSetting(setting, resuming, subject, owner)
			if loadErr != nil {
				return loadErr
			}
			cfg = &loaded
			settingSources = sources
		}
		if cfg != nil && cfg.Has("NORNS_ENABLE") {
			useNorns = cfg.NornsEnable
		}
		if resuming {
			requested := subject.Profile
			if cmd.Flags().Changed("norns") || cfg != nil && cfg.Has("NORNS_ENABLE") {
				requested = runtimeProfile(useNorns)
			}
			if cmd.Flags().Changed("cc") && subject.Profile.Agent != "cc" || !requested.Equal(subject.Profile) {
				return domain.NewError("RUNTIME_PROFILE_MISMATCH", "requested profile does not match subject", http.StatusConflict)
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
		if (!resuming && useNorns) || (resuming && subject.Profile.Has("norns")) {
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
		if setting == bareSetting {
			if err = waitForAutoSettingLaunch(cmd.Context(), stderr, settingSources); err != nil {
				return err
			}
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
		settingsPath, configHash, err := config.Publish(subject.Profile.Agent, path, runtimeID, cfg)
		if err != nil {
			return err
		}
		customPluginPath, customPluginHash, err := config.PublishCustom(path, runtimeID, cfg)
		if err != nil {
			return err
		}
		pluginPaths := make([]string, 0, 2)
		if cfg != nil {
			pluginPaths = append(pluginPaths, cfg.CustomPluginDirs...)
		}
		if customPluginPath != "" {
			pluginPaths = append(pluginPaths, customPluginPath)
		}
		pluginHash := ""
		rootAgent := ""
		if subject.Profile.Has("norns") {
			publication, publishErr := plugin.Publish(subject.Profile.Agent, path, runtimeID, cfg)
			err = publishErr
			if err != nil {
				return err
			}
			pluginPaths = append(pluginPaths, publication.Path)
			pluginHash = publication.Hash
			rootAgent = publication.RootAgent
		}
		runtime, resumeSessionID, err := owner.Store.CreateDirectRuntimeWithSession(cmd.Context(), subject.ID, runtimeID, resumeSession, time.Now().UTC())
		if err != nil {
			return err
		}
		launch := ccagent.Launch{Exec: execArgs, Passthrough: args, SessionID: runtimeID, ResumeSessionID: resumeSessionID, SettingsPath: settingsPath, PluginPaths: pluginPaths, RootAgent: rootAgent, Dir: path}
		createdAt := time.Now().UTC()
		snapshot, publishErr := runtimeartifact.Publish(path, runtimeartifact.Input{Subject: subject, Runtime: runtime, Launch: launch, Managed: false, Config: cfg, ConfigHash: configHash, PluginHash: pluginHash, CustomPluginHash: customPluginHash, CreatedAt: createdAt})
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
			go func() {
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
		_ = owner.Store.SetRuntimeTerminalWithDetails(context.Background(), subject.ID, runtimeID, state, reason, code, errorDetails, time.Now().UTC(), true)
		_, _ = logger.Event(context.Background(), "runtime.finished", "runtime", map[string]any{"state": state, "exit_code": result.ExitCode}, time.Now().UTC())
		if norns != nil {
			_, _ = fmt.Fprintln(stderr, "提示：Subject 的 .nornsd/ 目录包含运行配置和 Agent transcript，可能包含访问凭据或敏感内容，请谨慎分享。")
		}
		if agentFailed && !userInterrupted.Load() {
			if agentErr == nil {
				agentErr = fmt.Errorf("Claude Code exited with status %d", result.ExitCode)
			}
			return domain.Wrap("AGENT_EXITED", "Claude Code exited with an error", http.StatusBadGateway, agentErr)
		}
		return finalErr
	}}
	cmd.Flags().BoolVar(&cc, "cc", false, "use Claude Code")
	cmd.Flags().BoolVar(&cx, "cx", false, "use Codex (not implemented)")
	cmd.Flags().BoolVar(&pi, "pi", false, "use Pi (not implemented)")
	cmd.Flags().BoolVar(&resumeSession, "resume-session", false, "fork the latest Runtime session into the new Runtime (requires --resume-subject)")
	cmd.Flags().StringVar(&resumeSubject, "resume-subject", "", "reuse a Subject UUID and create a new Runtime")
	cmd.Flags().BoolVar(&useNorns, "norns", false, "enable Norns")
	cmd.Flags().StringVar(&setting, "setting", "", "generate Agent settings")
	cmd.Flags().Lookup("setting").NoOptDefVal = bareSetting
	return cmd
}

func loadAgentSetting(value string, resuming bool, subject domain.Subject, owner *root.Owner) (config.Config, []string, error) {
	if value != bareSetting {
		cfg, err := config.Load(value)
		if err != nil {
			return cfg, nil, domain.Wrap("INVALID_SETTING", "cannot load agent setting", http.StatusBadRequest, err)
		}
		path, pathErr := filepath.Abs(value)
		if pathErr != nil {
			return cfg, nil, pathErr
		}
		return cfg, []string{path}, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return config.Config{}, nil, err
	}
	paths := []string{filepath.Join(filepath.Dir(exe), "agent.yaml")}
	if resuming {
		subjectPath, _ := owner.SubjectPath(subject.DirectoryName)
		paths = append(paths, filepath.Join(subjectPath, ".nornsd", "config", "agent.yaml"))
	}
	var merged config.Config
	var sources []string
	found := false
	for _, path := range paths {
		if _, statErr := os.Stat(path); errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		layer, loadErr := config.LoadPartial(path)
		if loadErr != nil {
			return merged, nil, domain.Wrap("INVALID_SETTING", "cannot load agent setting", http.StatusBadRequest, loadErr)
		}
		absolute, pathErr := filepath.Abs(path)
		if pathErr != nil {
			return merged, nil, pathErr
		}
		sources = append(sources, absolute)
		if !found {
			merged = layer
			found = true
		} else {
			merged = config.Merge(merged, layer)
		}
	}
	if !found {
		return merged, nil, domain.NewError("SETTING_NOT_FOUND", "no default agent.yaml was found", http.StatusBadRequest)
	}
	merged, err = config.Normalize(merged)
	if err != nil {
		return merged, nil, domain.Wrap("INVALID_SETTING", "invalid merged agent setting", http.StatusBadRequest, err)
	}
	return merged, sources, nil
}

func waitForAutoSettingLaunch(ctx context.Context, stderr io.Writer, sources []string) error {
	_, _ = fmt.Fprintln(stderr, "nornsd: 已自动发现并读取以下 Runtime 配置（后者覆盖前者）：")
	for i, source := range sources {
		_, _ = fmt.Fprintf(stderr, "  %d. %s\n", i+1, source)
	}
	_, _ = fmt.Fprintln(stderr, "nornsd: 3 秒后启动 Claude Code，按 Ctrl+C 取消。")

	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	defer signal.Stop(interrupt)
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		_, _ = fmt.Fprintln(stderr, "nornsd: 已取消启动。")
		return errLaunchCancelled
	case <-interrupt:
		_, _ = fmt.Fprintln(stderr, "nornsd: 已取消启动。")
		return errLaunchCancelled
	}
}
func runtimeProfile(useNorns bool) domain.RuntimeProfile {
	capabilities := []string{}
	if useNorns {
		capabilities = []string{"norns"}
	}
	return domain.RuntimeProfile{Agent: "cc", AgentVersion: version.ClaudeCodeVersion, Capabilities: capabilities}
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
