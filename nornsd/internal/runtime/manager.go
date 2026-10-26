package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	ccagent "github.com/Ghr07h/norns/nornsd/internal/agent/cc"
	"github.com/Ghr07h/norns/nornsd/internal/config"
	"github.com/Ghr07h/norns/nornsd/internal/domain"
	"github.com/Ghr07h/norns/nornsd/internal/eventlog"
	nornsadapter "github.com/Ghr07h/norns/nornsd/internal/norns"
	"github.com/Ghr07h/norns/nornsd/internal/plugin"
	"github.com/Ghr07h/norns/nornsd/internal/process"
	"github.com/Ghr07h/norns/nornsd/internal/root"
	"github.com/Ghr07h/norns/nornsd/internal/runtimeartifact"
	"github.com/Ghr07h/norns/nornsd/internal/scheduler"
	storesqlite "github.com/Ghr07h/norns/nornsd/internal/store/sqlite"
	"github.com/Ghr07h/norns/nornsd/internal/version"
)

const memoryLimitExceededCode = "RUNTIME_MEMORY_LIMIT_EXCEEDED"

type Options struct {
	Owner       *root.Owner
	Profile     domain.RuntimeProfile
	Concurrency int
	SlotConfigs []*config.Config // indexed by stable slot number; element 0 is ignored
	Norns       *nornsadapter.Adapter
	MemoryLimit uint64
	OnError     func(error)
}

type Manager struct {
	ctx         context.Context
	cancel      context.CancelFunc
	owner       *root.Owner
	store       *storesqlite.Store
	profile     domain.RuntimeProfile
	configs     []*config.Config
	norns       *nornsadapter.Adapter
	memoryLimit uint64
	onError     func(error)
	scheduler   *scheduler.Scheduler

	mu               sync.RWMutex
	active           map[string]*managedRuntime
	closing          bool
	forceClosing     bool
	lifecycle        sync.WaitGroup
	closeOnce        sync.Once
	projectorHealthy atomic.Bool
}

type managedRuntime struct {
	subject         domain.Subject
	runtime         domain.Runtime
	resumeSessionID string
	slot            int
	path            string
	session         *ccagent.Session
	logger          *eventlog.Logger
	wake            chan struct{}
	done            chan struct{}
	exited          chan struct{}
	first           bool
	instructionSent bool
	resumeStartup   bool
	recoveryNotice  string
	finish          sync.Once
	closeOnce       sync.Once
	protocolMu      sync.Mutex
	identity        process.Identity
}

func NewManager(parent context.Context, opts Options) *Manager {
	ctx, cancel := context.WithCancel(parent)
	m := &Manager{ctx: ctx, cancel: cancel, owner: opts.Owner, store: opts.Owner.Store, profile: opts.Profile, configs: opts.SlotConfigs, norns: opts.Norns, memoryLimit: opts.MemoryLimit, onError: opts.OnError, active: map[string]*managedRuntime{}}
	m.projectorHealthy.Store(true)
	m.scheduler = scheduler.New(ctx, opts.Concurrency, m.claim, opts.OnError)
	return m
}

func (m *Manager) Start()       { m.scheduler.Start() }
func (m *Manager) NotifyQueue() { m.scheduler.Notify() }
func (m *Manager) Healthy() bool {
	m.mu.RLock()
	closing := m.closing
	m.mu.RUnlock()
	return !closing && m.projectorHealthy.Load() && m.scheduler.Healthy()
}

func (m *Manager) claim(ctx context.Context, slot int) (bool, error) {
	m.mu.Lock()
	if m.closing {
		m.mu.Unlock()
		return false, nil
	}
	// Register the claim before touching the queue so shutdown also waits for
	// runtimes that have been claimed but are not visible in active yet.
	m.lifecycle.Add(1)
	m.mu.Unlock()

	subject, runtime, found, err := m.store.ClaimNext(ctx, slot, time.Now().UTC())
	if err != nil || !found {
		m.lifecycle.Done()
		return found, err
	}
	go func() {
		defer m.lifecycle.Done()
		m.launch(subject, runtime, slot)
	}()
	return true, nil
}

func (m *Manager) launch(subject domain.Subject, r domain.Runtime, slot int) {
	resumeSessionID, err := runtimeResumeSessionID(r)
	if err != nil {
		m.failBeforeStart(subject, r, slot, "RUNTIME_SESSION_NOT_FOUND", err)
		return
	}
	path, err := m.owner.MaterializeSubject(m.ctx, subject)
	if err != nil {
		m.failBeforeStart(subject, r, slot, "WORKSPACE_UNAVAILABLE", err)
		return
	}
	var cfg *config.Config
	if slot < len(m.configs) {
		cfg = m.configs[slot]
	}
	settingsPath, configHash, err := config.Publish(subject.Profile.Agent, path, r.ID, cfg)
	if err != nil {
		m.failBeforeStart(subject, r, slot, "INVALID_SETTING", err)
		return
	}
	customPluginPath, customPluginHash, err := config.PublishCustom(path, r.ID, cfg)
	if err != nil {
		m.failBeforeStart(subject, r, slot, "CUSTOM_PUBLICATION_FAILED", err)
		return
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
	if m.profile.Has("norns") {
		publication, publishErr := plugin.Publish(subject.Profile.Agent, path, r.ID, cfg)
		err = publishErr
		if err != nil {
			m.failBeforeStart(subject, r, slot, "PLUGIN_GENERATION_FAILED", err)
			return
		}
		pluginPaths = append(pluginPaths, publication.Path)
		pluginHash = publication.Hash
		rootAgent = publication.RootAgent
	}
	execArgs := []string{"claude"}
	strict := true
	if cfg != nil {
		execArgs = append([]string(nil), cfg.AgentExec...)
		strict = ccagent.RequiresExactVersion(cfg.AgentExec, cfg.AgentExecExplicit())
	}
	logger := eventlog.New(m.store, path, subject.ID, r.ID)
	if err = eventlog.SyncReadable(m.ctx, m.store, path, subject.ID); err != nil {
		m.failBeforeStart(subject, r, slot, "EVENT_PROJECTOR_FAILED", err)
		return
	}
	launch := ccagent.Launch{Exec: execArgs, SessionID: r.ID, ResumeSessionID: resumeSessionID, SettingsPath: settingsPath, PluginPaths: pluginPaths, RootAgent: rootAgent, Dir: path}
	createdAt := time.Now().UTC()
	snapshot, err := runtimeartifact.Publish(path, runtimeartifact.Input{Subject: subject, Runtime: r, Launch: launch, Managed: true, Config: cfg, ConfigHash: configHash, PluginHash: pluginHash, CustomPluginHash: customPluginHash, Slot: &slot, CreatedAt: createdAt})
	if err != nil {
		m.failBeforeStart(subject, r, slot, "RUNTIME_SNAPSHOT_FAILED", err)
		return
	}
	if err = m.store.SaveConfigSnapshot(m.ctx, r.ID, snapshot, createdAt); err != nil {
		m.failBeforeStart(subject, r, slot, "RUNTIME_SNAPSHOT_FAILED", err)
		return
	}
	if _, err = logger.Record(m.ctx, "runtime", "lifecycle", json.RawMessage(`{"phase":"launching"}`), time.Now().UTC()); err != nil {
		m.projectorHealthy.Store(false)
		m.failBeforeStart(subject, r, slot, "EVENT_PROJECTOR_FAILED", err)
		return
	}
	recoveryNotice, previousRuntimeID, noticeErr := m.memoryRecoveryNotice(subject.ID, r.ID, memoryRecoveryNoticeEnabled(subject.Profile, resumeSessionID))
	if noticeErr != nil {
		m.report(fmt.Errorf("load runtime recovery notice: %w", noticeErr))
	}
	mr := &managedRuntime{subject: subject, runtime: r, resumeSessionID: resumeSessionID, slot: slot, path: path, logger: logger, wake: make(chan struct{}, 1), done: make(chan struct{}), exited: make(chan struct{}), first: true, resumeStartup: resumeSessionID != "", recoveryNotice: recoveryNotice}
	session, err := ccagent.StartManaged(m.ctx, launch, logger.RawHandler(m.ctx), func(raw json.RawMessage) error { return m.handleMessage(mr, raw) })
	if err != nil {
		m.failBeforeStart(subject, r, slot, "AGENT_UNAVAILABLE", err)
		return
	}
	mr.session = session
	identity, identityErr := session.Identity()
	if identityErr != nil {
		ctx, cancel := context.WithTimeout(context.Background(), version.ControlTimeout)
		_ = session.Terminate(ctx)
		cancel()
		_, _ = session.Wait()
		m.failBeforeStart(subject, r, slot, "PROCESS_IDENTITY_UNAVAILABLE", identityErr)
		return
	}
	mr.identity = identity
	resourceControl := session.ResourceControl()
	if recoveryNotice != "" {
		_, _ = logger.Event(context.Background(), "runtime.recovery_notice", "runtime", map[string]any{"code": "PREVIOUS_RUNTIME_MEMORY_LIMIT_EXCEEDED", "previous_runtime_id": previousRuntimeID}, time.Now().UTC())
	}
	m.mu.Lock()
	m.active[r.ID] = mr
	closing := m.closing
	forceClosing := m.forceClosing
	m.mu.Unlock()
	m.run(func() { m.wait(mr) })
	m.run(func() { m.monitor(mr) })
	if closing {
		mode := "graceful"
		if forceClosing {
			mode = "force"
		}
		_, _, _ = m.store.RequestStop(context.Background(), subject.ID, "server shutdown", time.Now().UTC())
		_ = m.Stop(r.ID, mode)
		return
	}
	processInfo := map[string]any{"identity": identity, "session_id": r.ID, "slot": slot, "strict_version": strict, "resource_control": resourceControl}
	if resumeSessionID != "" {
		processInfo["resume_session_id"] = resumeSessionID
	}
	if err = m.store.UpdateRuntimeLaunch(m.ctx, subject.ID, r.ID, configHash, processInfo, time.Now().UTC()); err != nil {
		m.failActive(mr, "PROCESS_IDENTITY_UNAVAILABLE", err)
		return
	}
	initCtx, cancel := context.WithTimeout(m.ctx, version.ControlTimeout)
	err = session.Initialize(initCtx)
	cancel()
	if err != nil {
		m.failActive(mr, "AGENT_PROTOCOL_ERROR", err)
		return
	}
	current, getErr := m.store.GetRuntime(m.ctx, subject.ID, r.ID, false)
	if getErr != nil || current.State.Terminal() {
		_ = session.Terminate(context.Background())
		return
	}
	m.run(func() { m.writer(mr) })
	m.wake(mr)
}

func (m *Manager) run(fn func()) {
	m.lifecycle.Add(1)
	go func() {
		defer m.lifecycle.Done()
		fn()
	}()
}

func (m *Manager) failBeforeStart(subject domain.Subject, r domain.Runtime, slot int, code string, cause error) {
	now := time.Now().UTC()
	_ = m.store.SetRuntimeTerminalWithDetails(context.Background(), subject.ID, r.ID, domain.RuntimeError, "runtime launch failed", code, map[string]any{"message": cause.Error()}, now, false)
	_, _ = m.store.AppendEvent(context.Background(), subject.ID, &r.ID, "runtime.error", "runtime", map[string]any{"code": code, "message": cause.Error()}, now)
	_ = m.store.FinalizeRuntime(context.Background(), subject.ID, r.ID, time.Now().UTC())
	if path, pathErr := m.owner.SubjectPath(subject.DirectoryName); pathErr == nil {
		if repairErr := eventlog.SyncReadable(context.Background(), m.store, path, subject.ID); repairErr != nil {
			m.projectorHealthy.Store(false)
		}
	}
	m.scheduler.Release(slot)
	m.report(cause)
}

func (m *Manager) failActive(mr *managedRuntime, code string, cause error) {
	now := time.Now().UTC()
	reason := runtimeFailureReason(code)
	_ = m.store.SetRuntimeTerminalWithDetails(context.Background(), mr.subject.ID, mr.runtime.ID, domain.RuntimeError, reason, code, map[string]any{"message": cause.Error()}, now, false)
	mr.logger.Diagnostic(context.Background(), code, cause)
	ctx, cancel := context.WithTimeout(context.Background(), version.ControlTimeout)
	_ = mr.session.Terminate(ctx)
	cancel()
}

func runtimeFailureReason(code string) string {
	if code == memoryLimitExceededCode {
		return memoryLimitExceededCode
	}
	return "runtime failed"
}

func (m *Manager) writer(mr *managedRuntime) {
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-mr.done:
			return
		case <-mr.wake:
			for {
				items, group, err := m.store.DispatchPending(m.ctx, mr.subject.ID, mr.runtime.ID, mr.first, time.Now().UTC())
				if err != nil {
					m.failActive(mr, "INSTRUCTION_DELIVERY_FAILED", err)
					return
				}
				if len(items) == 0 {
					break
				}
				content := items[0].Content
				if mr.first && len(items) > 1 {
					parts := make([]string, len(items))
					for i, item := range items {
						parts[i] = item.Content
					}
					content = strings.Join(parts, "\n\n")
				}
				if mr.first && mr.recoveryNotice != "" {
					content = mr.recoveryNotice + "\n\n" + content
				}
				mr.first = false
				mr.protocolMu.Lock()
				if err = mr.session.SendUser(content); err != nil {
					mr.protocolMu.Unlock()
					m.failActive(mr, "INSTRUCTION_DELIVERY_FAILED", err)
					return
				}
				if err = m.store.ConfirmDispatch(m.ctx, mr.subject.ID, mr.runtime.ID, group, time.Now().UTC()); err != nil {
					mr.protocolMu.Unlock()
					m.failActive(mr, "INSTRUCTION_DELIVERY_FAILED", err)
					return
				}
				mr.instructionSent = true
				if err = eventlog.SyncReadable(m.ctx, m.store, mr.path, mr.subject.ID); err != nil {
					mr.protocolMu.Unlock()
					m.projectorHealthy.Store(false)
					m.failActive(mr, "EVENT_PROJECTOR_FAILED", err)
					return
				}
				mr.protocolMu.Unlock()
			}
		}
	}
}

func (m *Manager) handleMessage(mr *managedRuntime, raw json.RawMessage) error {
	mr.protocolMu.Lock()
	defer mr.protocolMu.Unlock()
	var envelope struct {
		Type   string `json:"type"`
		Origin struct {
			Kind string `json:"kind"`
		} `json:"origin"`
	}
	parsed := json.Unmarshal(raw, &envelope) == nil
	taskNotificationResult := parsed && envelope.Type == "result" && envelope.Origin.Kind == "task-notification"
	if mr.resumeStartup && parsed && envelope.Type == "result" && (!mr.instructionSent || taskNotificationResult) {
		if err := mr.logger.RecordSessionResumed(m.ctx, raw, mr.resumeSessionID, time.Now().UTC()); err != nil {
			m.projectorHealthy.Store(false)
			return fmt.Errorf("event projector: %w", err)
		}
		return nil
	}
	if mr.resumeStartup && parsed && (envelope.Type == "assistant" || envelope.Type == "stream_event" || (envelope.Type == "result" && !taskNotificationResult)) {
		mr.resumeStartup = false
	}
	result, err := mr.logger.Record(m.ctx, "stdout", "agent_message", raw, time.Now().UTC())
	if err != nil {
		m.projectorHealthy.Store(false)
		return fmt.Errorf("event projector: %w", err)
	}
	if result.CloseSucceeded && m.profile.Has("norns") {
		mr.closeOnce.Do(func() {
			now := time.Now().UTC()
			deadline := now.Add(version.NornsCloseGrace)
			if m.store.SetClosing(m.ctx, mr.subject.ID, mr.runtime.ID, deadline, now) == nil {
				if _, eventErr := mr.logger.Event(m.ctx, "norns.lifecycle", "norns", map[string]any{"phase": "close_pending", "deadline": deadline}, now); eventErr != nil {
					err = eventErr
					return
				}
				go m.closeDeadline(mr, deadline)
			}
		})
		if err != nil {
			m.projectorHealthy.Store(false)
			return err
		}
	}
	// Claude emits an extra result for asynchronous task notifications. Depending
	// on whether the task belongs to this process or a resumed process, that
	// result may have non-zero turns and usage. Its origin, rather than those
	// counters, identifies it as a notification instead of an instruction boundary.
	if !result.IsResult || taskNotificationResult {
		return nil
	}
	updated, err := m.store.CompleteTurn(m.ctx, mr.subject.ID, mr.runtime.ID, result.Usage, time.Now().UTC())
	if err != nil {
		return err
	}
	if updated.State == domain.RuntimeWaiting && updated.Closing {
		_ = m.store.SetRuntimeTerminal(context.Background(), mr.subject.ID, mr.runtime.ID, domain.RuntimeEnded, "norns task closed", "", time.Now().UTC(), false)
		_ = mr.session.CloseInput()
		return nil
	}
	m.wake(mr)
	return nil
}

func (m *Manager) closeDeadline(mr *managedRuntime, deadline time.Time) {
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case <-mr.done:
		return
	case <-timer.C:
		current, err := m.store.GetRuntime(context.Background(), mr.subject.ID, mr.runtime.ID, false)
		if err == nil && !current.State.Terminal() {
			_ = m.store.SetRuntimeTerminal(context.Background(), mr.subject.ID, mr.runtime.ID, domain.RuntimeError, "Norns close grace period expired", "CLOSE_GRACE_TIMEOUT", time.Now().UTC(), false)
			ctx, cancel := context.WithTimeout(context.Background(), version.ControlTimeout)
			_ = mr.session.Terminate(ctx)
			cancel()
		}
	}
}

func (m *Manager) wait(mr *managedRuntime) {
	result, waitErr := mr.session.Wait()
	close(mr.exited)
	mr.finish.Do(func() { m.finishRuntime(mr, result, waitErr) })
}

func (m *Manager) finishRuntime(mr *managedRuntime, result process.Result, waitErr error) {
	now := time.Now().UTC()
	current, err := m.store.GetRuntime(context.Background(), mr.subject.ID, mr.runtime.ID, false)
	if err == nil && !current.State.Terminal() {
		code := "AGENT_EXITED"
		reason := "agent process exited"
		if waitErr != nil && strings.Contains(waitErr.Error(), "agent protocol") {
			code = "AGENT_PROTOCOL_ERROR"
		}
		details := map[string]any{"message": reason, "exit_code": result.ExitCode, "signal": result.Signal}
		if waitErr != nil {
			details["cause"] = waitErr.Error()
		}
		_ = m.store.SetRuntimeTerminalWithDetails(context.Background(), mr.subject.ID, mr.runtime.ID, domain.RuntimeError, reason, code, details, now, false)
		current.State = domain.RuntimeError
		current.ErrorCode = &code
	}
	processInfo := map[string]any{"identity": mr.identity, "session_id": mr.runtime.ID, "result": result}
	if mr.resumeSessionID != "" {
		processInfo["resume_session_id"] = mr.resumeSessionID
	}
	_ = m.store.UpdateRuntimeProcess(context.Background(), mr.subject.ID, mr.runtime.ID, processInfo)
	resource := map[string]any{}
	if len(current.ResourceJSON) > 0 {
		_ = json.Unmarshal(current.ResourceJSON, &resource)
	}
	if len(resource) > 0 {
		resource["memory_current_bytes"] = uint64(0)
		_ = m.store.UpdateRuntimeResource(context.Background(), mr.subject.ID, mr.runtime.ID, resource)
	}
	transcript, archiveErr := ccagent.ArchiveTranscript(mr.path, mr.runtime.ID, mr.runtime.ID)
	if archiveErr != nil {
		transcript.Status = "failed"
		mr.logger.Diagnostic(context.Background(), "TRANSCRIPT_ARCHIVE_FAILED", archiveErr)
	}
	_ = m.store.UpdateRuntimeTranscript(context.Background(), mr.subject.ID, mr.runtime.ID, transcript)
	if m.profile.Has("norns") && m.norns != nil && (current.State == domain.RuntimeCancelled || current.State == domain.RuntimeError) {
		outcome, reason := "failed", "Runtime failed under nornsd control"
		if current.State == domain.RuntimeCancelled {
			outcome, reason = "cancelled", "Runtime terminated externally by nornsd"
		} else if current.ErrorCode != nil && *current.ErrorCode == memoryLimitExceededCode {
			reason = memoryLimitExceededCode
		}
		ctx, cancel := context.WithTimeout(context.Background(), version.ControlTimeout)
		_, cleanupErr := m.norns.Cleanup(ctx, mr.path, outcome, reason)
		cancel()
		if cleanupErr != nil {
			mr.logger.Diagnostic(context.Background(), "NORNS_CLEANUP_FAILED", cleanupErr)
		}
	}
	if waitErr != nil && strings.Contains(waitErr.Error(), "event projector") {
		m.projectorHealthy.Store(false)
	}
	exitPayload, _ := json.Marshal(map[string]any{"phase": "exited", "exit_code": result.ExitCode, "wait_error": waitErr != nil})
	if _, recordErr := mr.logger.Record(context.Background(), "runtime", "lifecycle", exitPayload, time.Now().UTC()); recordErr != nil {
		m.projectorHealthy.Store(false)
	}
	if repairErr := eventlog.SyncReadable(context.Background(), m.store, mr.path, mr.subject.ID); repairErr != nil {
		m.projectorHealthy.Store(false)
	}
	if _, eventErr := mr.logger.Event(context.Background(), "runtime.finished", "runtime", map[string]any{"state": current.State, "exit_code": result.ExitCode, "wait_error": waitErr != nil}, time.Now().UTC()); eventErr != nil {
		m.projectorHealthy.Store(false)
	}
	_ = m.store.FinalizeRuntime(context.Background(), mr.subject.ID, mr.runtime.ID, time.Now().UTC())
	m.mu.Lock()
	delete(m.active, mr.runtime.ID)
	close(mr.done)
	m.mu.Unlock()
	m.scheduler.Release(mr.slot)
}

func (m *Manager) monitor(mr *managedRuntime) {
	samples, err := openMemorySampleWriter(mr.path, mr.runtime.ID)
	if err != nil {
		m.report(fmt.Errorf("open Runtime memory samples: %w", err))
	} else {
		defer samples.Close()
	}
	ticker := time.NewTicker(version.ResourceSampleInterval)
	defer ticker.Stop()
	var peak uint64
	accounting := mr.session.ResourceControl().MemoryAccounting
	sample := func() bool {
		current, err := mr.session.RSS()
		if err != nil {
			m.failActive(mr, "RESOURCE_MONITOR_FAILED", err)
			return false
		}
		if current > peak {
			peak = current
		}
		sampledAt := time.Now().UTC()
		if samples != nil {
			if writeErr := samples.Write(memorySample{SampledAt: sampledAt, MemoryBytes: current}); writeErr != nil {
				m.report(fmt.Errorf("write Runtime memory sample: %w", writeErr))
				_ = samples.Close()
				samples = nil
			}
		}
		resource := map[string]any{"memory_current_bytes": current, "memory_peak_bytes": peak, "memory_limit_bytes": m.memoryLimit, "memory_accounting": accounting, "memory_sample_interval_ms": version.ResourceSampleInterval.Milliseconds()}
		_ = m.store.UpdateRuntimeResource(context.Background(), mr.subject.ID, mr.runtime.ID, resource)
		if m.memoryLimit > 0 && current > m.memoryLimit {
			m.failActive(mr, memoryLimitExceededCode, fmt.Errorf("runtime memory %d exceeds limit %d", current, m.memoryLimit))
			return false
		}
		return true
	}
	select {
	case <-mr.exited:
		return
	default:
		if !sample() {
			return
		}
	}
	for {
		select {
		case <-mr.exited:
			return
		case <-ticker.C:
			if !sample() {
				return
			}
		}
	}
}

func (m *Manager) memoryRecoveryNotice(subjectID, runtimeID string, enabled bool) (string, string, error) {
	if !enabled {
		return "", "", nil
	}
	runtimes, err := m.store.ListRuntimes(m.ctx, subjectID, 4, "", "")
	if err != nil {
		return "", "", err
	}
	message, previousRuntimeID := memoryRecoveryNoticeFor(runtimes, runtimeID)
	return message, previousRuntimeID, nil
}

func memoryRecoveryNoticeEnabled(profile domain.RuntimeProfile, resumeSessionID string) bool {
	return profile.Has("norns") || resumeSessionID != ""
}

func memoryRecoveryNoticeFor(runtimes []domain.Runtime, runtimeID string) (string, string) {
	for _, previous := range runtimes {
		if previous.ID == runtimeID {
			continue
		}
		if previous.ErrorCode == nil || *previous.ErrorCode != memoryLimitExceededCode {
			return "", ""
		}
		message := "之前的智能体运行时因执行过程中内存异常增长被终止，请评估并避免重复触发该问题后继续执行当前指令。"
		return message, previous.ID
	}
	return "", ""
}

func (m *Manager) Wake(runtimeID string) {
	m.mu.RLock()
	mr := m.active[runtimeID]
	m.mu.RUnlock()
	if mr != nil {
		m.wake(mr)
	}
}
func (m *Manager) wake(mr *managedRuntime) {
	select {
	case mr.wake <- struct{}{}:
	default:
	}
}

func (m *Manager) Stop(runtimeID, mode string) error {
	m.mu.RLock()
	mr := m.active[runtimeID]
	m.mu.RUnlock()
	if mr == nil {
		return nil
	}
	go func() {
		if mode == "graceful" {
			ctx, cancel := context.WithTimeout(context.Background(), version.ControlTimeout)
			err := mr.session.Interrupt(ctx)
			cancel()
			if err == nil {
				_ = mr.session.CloseInput()
				timer := time.NewTimer(version.GracefulStopTimeout)
				select {
				case <-mr.done:
					timer.Stop()
					return
				case <-timer.C:
				}
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), version.ControlTimeout)
		_ = mr.session.Terminate(ctx)
		cancel()
	}()
	return nil
}

func (m *Manager) Drain(force bool) {
	m.mu.Lock()
	if m.closing && (!force || m.forceClosing) {
		m.mu.Unlock()
		return
	}
	m.closing = true
	if force {
		m.forceClosing = true
	}
	items := make([]*managedRuntime, 0, len(m.active))
	for _, mr := range m.active {
		items = append(items, mr)
	}
	m.mu.Unlock()
	m.scheduler.Drain()
	mode := "graceful"
	if force {
		mode = "force"
	}
	for _, mr := range items {
		_, _, _ = m.store.RequestStop(context.Background(), mr.subject.ID, "server shutdown", time.Now().UTC())
		_ = m.Stop(mr.runtime.ID, mode)
	}
}

func (m *Manager) Wait() {
	m.lifecycle.Wait()
}
func (m *Manager) Close() {
	m.closeOnce.Do(func() {
		m.Drain(true)
		m.cancel()
		m.scheduler.Close()
		m.lifecycle.Wait()
	})
}
func (m *Manager) report(err error) {
	if err != nil && m.onError != nil {
		m.onError(err)
	}
}

func (m *Manager) Recover(ctx context.Context) error {
	if err := m.recoverSubjects(ctx); err != nil {
		return err
	}
	runtimes, err := m.store.CurrentRuntimes(ctx)
	if err != nil {
		return err
	}
	for _, r := range runtimes {
		if r.State == domain.RuntimeQueued {
			continue
		}
		now := time.Now().UTC()
		if _, uncertainErr := m.store.MarkUncertainDeliveries(ctx, r.SubjectID, r.ID, now); uncertainErr != nil {
			return uncertainErr
		}
		identity, identified := persistedIdentity(r.ProcessJSON)
		status := process.RecoveryUnknown
		var recoveryErr error
		if identified {
			recoveryCtx, cancel := context.WithTimeout(ctx, version.ControlTimeout)
			status, recoveryErr = process.RecoverTree(recoveryCtx, identity)
			cancel()
		}
		if !identified || status == process.RecoveryUnknown || recoveryErr != nil {
			if !r.State.Terminal() {
				errorDetails := map[string]any{"message": "server interrupted; process identity could not be confirmed", "recovery_status": status}
				if recoveryErr != nil {
					errorDetails["cause"] = recoveryErr.Error()
				}
				_ = m.store.SetRuntimeTerminalWithDetails(ctx, r.SubjectID, r.ID, domain.RuntimeError, "server interrupted; process identity could not be confirmed", "SERVER_INTERRUPTED", errorDetails, now, false)
				_, _ = m.store.AppendEvent(ctx, r.SubjectID, &r.ID, "runtime.error", "runtime", map[string]any{"code": "SERVER_INTERRUPTED", "recovery_blocked": true}, now)
			}
			detail := map[string]any{"identity": identity, "recovery": map[string]any{"status": "blocked", "reason": "process_identity_unconfirmed"}}
			if sessionID := recoveredSessionID(r, ""); sessionID != "" {
				detail["session_id"] = sessionID
			}
			if resumeSessionID, resumeErr := runtimeResumeSessionID(r); resumeErr == nil && resumeSessionID != "" {
				detail["resume_session_id"] = resumeSessionID
			}
			if recoveryErr != nil {
				detail["recovery"].(map[string]any)["error"] = recoveryErr.Error()
			}
			_ = m.store.UpdateRuntimeProcess(ctx, r.SubjectID, r.ID, detail)
			_ = m.repairReadable(ctx, r.SubjectID)
			continue
		}
		if !r.State.Terminal() {
			_ = m.store.SetRuntimeTerminalWithDetails(ctx, r.SubjectID, r.ID, domain.RuntimeError, "server interrupted", "SERVER_INTERRUPTED", map[string]any{"message": "server interrupted", "recovery_status": status}, now, false)
			_, _ = m.store.AppendEvent(ctx, r.SubjectID, &r.ID, "runtime.error", "runtime", map[string]any{"code": "SERVER_INTERRUPTED", "process_recovery": status}, now)
		}
		subject, subjectErr := m.store.GetSubject(ctx, r.SubjectID)
		if subjectErr == nil {
			if path, pathErr := m.owner.SubjectPath(subject.DirectoryName); pathErr == nil {
				sessionID := recoveredSessionID(r, subject.SessionID)
				transcript, archiveErr := ccagent.ArchiveTranscript(path, sessionID, r.ID)
				if archiveErr != nil {
					transcript.Status = "failed"
				}
				_ = m.store.UpdateRuntimeTranscript(ctx, subject.ID, r.ID, transcript)
				if subject.Profile.Has("norns") && m.norns != nil {
					cleanupCtx, cancel := context.WithTimeout(ctx, version.ControlTimeout)
					_, _ = m.norns.Cleanup(cleanupCtx, path, "failed", "Runtime interrupted by nornsd restart")
					cancel()
				}
			}
		}
		if err = m.store.FinalizeRuntime(ctx, r.SubjectID, r.ID, now); err != nil {
			return err
		}
		_ = m.repairReadable(ctx, r.SubjectID)
	}
	return nil
}

func recoveredSessionID(runtime domain.Runtime, legacySubjectSessionID string) string {
	var processData struct {
		SessionID string `json:"session_id"`
	}
	if json.Unmarshal(runtime.ProcessJSON, &processData) == nil && processData.SessionID != "" {
		return processData.SessionID
	}
	// Runtime records created before per-Runtime sessions did not persist the
	// launch session ID and always used the Subject session.
	return legacySubjectSessionID
}

func runtimeResumeSessionID(runtime domain.Runtime) (string, error) {
	if runtime.SessionMode != "resume" {
		return "", nil
	}
	var processData struct {
		ResumeSessionID string `json:"resume_session_id"`
	}
	if json.Unmarshal(runtime.ProcessJSON, &processData) == nil && processData.ResumeSessionID != "" {
		return processData.ResumeSessionID, nil
	}
	return "", fmt.Errorf("Runtime %s does not identify the session to resume", runtime.ID)
}

func (m *Manager) repairReadable(ctx context.Context, subjectID string) error {
	subject, err := m.store.GetSubject(ctx, subjectID)
	if err != nil {
		return err
	}
	path, err := m.owner.SubjectPath(subject.DirectoryName)
	if err != nil {
		return err
	}
	err = eventlog.SyncReadable(ctx, m.store, path, subjectID)
	if err != nil {
		m.projectorHealthy.Store(false)
	}
	return err
}

func (m *Manager) RepairReadable(ctx context.Context, subjectID string) error {
	return m.repairReadable(ctx, subjectID)
}

func persistedIdentity(raw json.RawMessage) (process.Identity, bool) {
	var value struct {
		Identity process.Identity `json:"identity"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil || value.Identity.PID <= 0 || value.Identity.StartIdentity == "" {
		return process.Identity{}, false
	}
	return value.Identity, true
}

func (m *Manager) recoverSubjects(ctx context.Context) error {
	subjects, _, err := m.store.ListSubjects(ctx, nil, 1_000_000, "", "")
	if err != nil {
		return err
	}
	byID := map[string]domain.Subject{}
	byDir := map[string]domain.Subject{}
	for _, subject := range subjects {
		byID[subject.ID] = subject
		byDir[subject.DirectoryName] = subject
	}
	entries, err := os.ReadDir(m.owner.Path)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == ".nornsd_root" {
			continue
		}
		path := filepath.Join(m.owner.Path, entry.Name())
		identity, readErr := root.ReadSubjectIdentity(path)
		if readErr != nil {
			continue
		}
		if existing, ok := byID[identity.SubjectID]; ok {
			if existing.DirectoryName != entry.Name() || !existing.Profile.Equal(identity.RuntimeProfile) {
				_ = m.store.MarkUnavailable(ctx, existing.ID, map[string]any{"code": "SUBJECT_IDENTITY_CONFLICT"}, time.Now().UTC())
			}
			continue
		}
		if conflict, ok := byDir[entry.Name()]; ok {
			_ = m.store.MarkUnavailable(ctx, conflict.ID, map[string]any{"code": "SUBJECT_DIRECTORY_CONFLICT"}, time.Now().UTC())
			continue
		}
		subject := domain.Subject{ID: identity.SubjectID, DirectoryName: entry.Name(), WorkspaceState: domain.WorkspaceMaterialized, Profile: identity.RuntimeProfile, SessionID: identity.SessionID, CreatedAt: identity.CreatedAt, UpdatedAt: time.Now().UTC()}
		if err = m.store.CreateSubject(ctx, subject, nil, "", identity.CreatedAt); err != nil {
			return err
		}
		if err = m.store.MarkMaterialized(ctx, subject.ID, time.Now().UTC()); err != nil {
			return err
		}
		_, _ = m.store.AppendEvent(ctx, subject.ID, nil, "diagnostic", "runtime", map[string]any{"code": "SUBJECT_REGISTRATION_RECOVERED"}, time.Now().UTC())
	}
	subjects, _, err = m.store.ListSubjects(ctx, nil, 1_000_000, "", "")
	if err != nil {
		return err
	}
	for _, subject := range subjects {
		if subject.WorkspaceState == domain.WorkspacePlanned && subject.State == domain.SubjectQueuing {
			continue
		}
		if subject.WorkspaceState == domain.WorkspaceMaterializing {
			if _, err = m.owner.MaterializeSubject(ctx, subject); err != nil {
				_ = m.store.MarkUnavailable(ctx, subject.ID, map[string]any{"code": "MATERIALIZATION_RECOVERY_FAILED"}, time.Now().UTC())
				continue
			}
		}
		if subject.WorkspaceState == domain.WorkspaceUnavailable {
			continue
		}
		path, pathErr := m.owner.SubjectPath(subject.DirectoryName)
		if pathErr != nil {
			_ = m.store.MarkUnavailable(ctx, subject.ID, map[string]any{"code": "INVALID_SUBJECT_PATH"}, time.Now().UTC())
			continue
		}
		identity, readErr := root.ReadSubjectIdentity(path)
		if readErr != nil || identity.SubjectID != subject.ID || identity.DirectoryName != subject.DirectoryName || identity.SessionID != subject.SessionID || !identity.RuntimeProfile.Equal(subject.Profile) {
			_ = m.store.MarkUnavailable(ctx, subject.ID, map[string]any{"code": "SUBJECT_IDENTITY_MISMATCH"}, time.Now().UTC())
			continue
		}
		if err = eventlog.ReplayRaw(ctx, m.store, path, subject.ID); err != nil {
			return err
		}
		if err = eventlog.RepairReadable(ctx, m.store, path, subject.ID); err != nil {
			return err
		}
		_ = os.RemoveAll(filepath.Join(path, ".nornsd", "staging"))
		if err = os.MkdirAll(filepath.Join(path, ".nornsd", "staging"), 0700); err != nil {
			return err
		}
	}
	_ = os.RemoveAll(filepath.Join(m.owner.Control, "staging"))
	return os.MkdirAll(filepath.Join(m.owner.Control, "staging"), 0700)
}
