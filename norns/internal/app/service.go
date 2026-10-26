package app

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Ghr07h/norns/norns/internal/domain"
	"github.com/Ghr07h/norns/norns/internal/i18n"
	"github.com/Ghr07h/norns/norns/internal/output"
	storesqlite "github.com/Ghr07h/norns/norns/internal/store/sqlite"
	"github.com/Ghr07h/norns/norns/internal/workspace"
)

type Service struct {
	Paths workspace.Paths
	Store *storesqlite.Store
	Lock  *workspace.LifecycleLock
	IDs   domain.IDGenerator
	Clock domain.Clock
	Files workspace.FileOps
}

func New(root string) (*Service, error) {
	paths, err := workspace.Resolve(root)
	if err != nil {
		return nil, err
	}
	ids := domain.RandomIDs{}
	clock := domain.SystemClock{}
	return &Service{Paths: paths, Store: storesqlite.New(paths, ids, clock), Lock: workspace.NewLock(paths), IDs: ids, Clock: clock, Files: workspace.OSFileOps{}}, nil
}

func (s *Service) shared(ctx context.Context, fn func(context.Context) error) error {
	for {
		lockCtx, cancel := workspace.TimeoutContext(ctx)
		unlock, err := s.Lock.Shared(lockCtx)
		cancel()
		if err != nil {
			return err
		}
		if _, err := os.Stat(s.Paths.ResetLog); errors.Is(err, os.ErrNotExist) {
			defer unlock()
			return fn(ctx)
		} else if err != nil {
			unlock()
			return domain.Wrap("IO_ERROR", "cannot inspect reset recovery log", err)
		}
		unlock()
		if err := s.exclusive(ctx, func(context.Context) error { return nil }); err != nil {
			return err
		}
	}
}

func (s *Service) exclusive(ctx context.Context, fn func(context.Context) error) error {
	lockCtx, cancel := workspace.TimeoutContext(ctx)
	defer cancel()
	unlock, err := s.Lock.Exclusive(lockCtx)
	if err != nil {
		return err
	}
	defer unlock()
	if _, err := os.Stat(s.Paths.ResetLog); err == nil {
		if err := s.recoverResetLocked(ctx); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return domain.Wrap("IO_ERROR", "cannot inspect reset recovery log", err)
	}
	return fn(ctx)
}

type Result struct {
	Data     any
	Warnings []output.Warning
	Message  string
}

func (s *Service) Init(ctx context.Context) (Result, error) {
	var result Result
	err := s.exclusive(ctx, func(ctx context.Context) error {
		if s.Store.Initialized() {
			return s.Store.Read(ctx, func(*sql.Tx) error {
				result = Result{Data: map[string]any{"initialized": false, "existing": true}, Message: "Workspace already exists"}
				return nil
			})
		}
		if err := s.Store.Initialize(ctx, nil); err != nil {
			return err
		}
		result = Result{Data: map[string]any{"initialized": true, "existing": false}, Message: "Workspace initialized"}
		return nil
	})
	return result, err
}

type OpenTaskRequest struct {
	Purpose     string
	Concurrency int
	Info        *string
}

func (s *Service) OpenTask(ctx context.Context, req OpenTaskRequest) (Result, error) {
	purpose, err := domain.NonEmpty(req.Purpose, "purpose")
	if err != nil {
		return Result{}, err
	}
	if req.Concurrency < 1 {
		return Result{}, domain.NewError("INVALID_ARGUMENT", "concurrency must be a positive integer")
	}
	var info *string
	if req.Info != nil {
		v, e := domain.NonEmpty(*req.Info, "info")
		if e != nil {
			return Result{}, e
		}
		info = &v
	}

	var result Result
	if !s.Store.Initialized() {
		err := s.exclusive(ctx, func(ctx context.Context) error {
			if s.Store.Initialized() {
				return s.openTaskExisting(ctx, purpose, req.Concurrency, info, &result)
			}
			if info == nil {
				return domain.NewError("INVALID_ARGUMENT", "info is required for the first Task")
			}
			return s.Store.Initialize(ctx, func(tx *sql.Tx) error {
				data, err := s.insertTask(ctx, tx, purpose, req.Concurrency, info)
				if err == nil {
					result = Result{Data: data, Message: "Task opened"}
				}
				return err
			})
		})
		return result, err
	}
	err = s.shared(ctx, func(ctx context.Context) error {
		return s.openTaskExisting(ctx, purpose, req.Concurrency, info, &result)
	})
	return result, err
}

func (s *Service) openTaskExisting(ctx context.Context, purpose string, concurrency int, info *string, result *Result) error {
	return s.Store.WriteQ(ctx, func(q storesqlite.Queryer) error {
		data, err := s.insertTask(ctx, q, purpose, concurrency, info)
		if err == nil {
			*result = Result{Data: data, Message: "Task opened"}
		}
		return err
	})
}

func (s *Service) insertTask(ctx context.Context, q storesqlite.Queryer, purpose string, concurrency int, info *string) (map[string]any, error) {
	var openCount, runtime int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM tasks WHERE status='open'`).Scan(&openCount); err != nil {
		return nil, dbErr(err)
	}
	if openCount != 0 {
		return nil, domain.NewError("OPEN_TASK_EXISTS", "Workspace already has an open Task")
	}
	if err := q.QueryRowContext(ctx, `SELECT runtime_occupied FROM workspace_meta WHERE singleton=1`).Scan(&runtime); err != nil {
		return nil, dbErr(err)
	}
	if runtime != 0 {
		return nil, domain.NewError("WORKSPACE_STATE_UNTRUSTED", "Runtime is occupied without an open Task")
	}
	var taskCount int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM tasks`).Scan(&taskCount); err != nil {
		return nil, dbErr(err)
	}
	if taskCount == 0 && info == nil {
		return nil, domain.NewError("INVALID_ARGUMENT", "info is required for the first Task")
	}

	taskID, err := s.IDs.TaskID()
	if err != nil {
		return nil, domain.Wrap("INTERNAL_ERROR", "cannot generate Task ID", err)
	}
	now := domain.TimeText(s.Clock.Now())
	initID := ""
	reused := false
	var cognitionID string
	if info == nil {
		if err := q.QueryRowContext(ctx, `SELECT init_cognition_ref FROM tasks WHERE status='closed' ORDER BY closed_at DESC, id ASC LIMIT 1`).Scan(&initID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, domain.NewError("NO_TASK", "no closed Task can provide Subject information")
			}
			return nil, dbErr(err)
		}
		reused = true
	} else {
		cognitionID, err = s.IDs.CognitionID()
		if err != nil {
			return nil, domain.Wrap("INTERNAL_ERROR", "cannot generate Cognition ID", err)
		}
		initID = cognitionID
	}
	if _, err := q.ExecContext(ctx, `INSERT INTO tasks(id,purpose,init_cognition_ref,max_concurrency,status,created_at) VALUES(?,?,?,?,'open',?)`, taskID, purpose, initID, concurrency, now); err != nil {
		return nil, entityInsertErr(err, "tasks")
	}
	if info != nil {
		var previous any
		var previousID string
		err := q.QueryRowContext(ctx, `SELECT id FROM cognitions WHERE origin='task' ORDER BY created_at DESC, id ASC LIMIT 1`).Scan(&previousID)
		if err == nil {
			previous = previousID
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, dbErr(err)
		}
		if _, err := q.ExecContext(ctx, `INSERT INTO cognitions(id,title,content,task_id,origin,origin_cognition_id,task_purpose_relevant,created_at) VALUES(?,?,?,?,'task',?,0,?)`, cognitionID, "Subject information", *info, taskID, previous, now); err != nil {
			return nil, entityInsertErr(err, "cognitions")
		}
	}
	if _, err := q.ExecContext(ctx, `UPDATE workspace_meta SET runtime_occupied=1 WHERE singleton=1 AND runtime_occupied=0`); err != nil {
		return nil, dbErr(err)
	}
	return map[string]any{"id": taskID, "init_cognition_ref": initID, "reused_init_cognition": reused, "created_at": now}, nil
}

func (s *Service) CloseTask(ctx context.Context, outcome, reason string) (Result, error) {
	if !domain.TaskOutcomes[outcome] {
		return Result{}, domain.NewError("INVALID_ARGUMENT", "invalid Task outcome")
	}
	reason, err := domain.NonEmpty(reason, "outcome-reason")
	if err != nil {
		return Result{}, err
	}
	var result Result
	err = s.shared(ctx, func(ctx context.Context) error {
		return s.Store.WriteQ(ctx, func(q storesqlite.Queryer) error {
			var taskID string
			if err := q.QueryRowContext(ctx, `SELECT id FROM tasks WHERE status='open'`).Scan(&taskID); err != nil {
				return noOpen(err)
			}
			now := domain.TimeText(s.Clock.Now())
			todoReason, runningReason := closeReasons()
			r1, err := q.ExecContext(ctx, `UPDATE explorations SET status='finished',outcome='cancelled',outcome_reason=?,finished_at=? WHERE task_id=? AND status='todo'`, todoReason, now, taskID)
			if err != nil {
				return dbErr(err)
			}
			r2, err := q.ExecContext(ctx, `UPDATE explorations SET status='finished',outcome='cancelled',outcome_reason=?,finished_at=? WHERE task_id=? AND status='running'`, runningReason, now, taskID)
			if err != nil {
				return dbErr(err)
			}
			if _, err := q.ExecContext(ctx, `UPDATE tasks SET status='closed',outcome=?,outcome_reason=?,closed_at=? WHERE id=? AND status='open'`, outcome, reason, now, taskID); err != nil {
				return dbErr(err)
			}
			if _, err := q.ExecContext(ctx, `UPDATE workspace_meta SET runtime_occupied=0 WHERE singleton=1`); err != nil {
				return dbErr(err)
			}
			a, _ := r1.RowsAffected()
			b, _ := r2.RowsAffected()
			result = Result{Data: map[string]any{"id": taskID, "outcome": outcome, "closed_at": now, "cancelled_todo": a, "cancelled_running": b}, Message: "Task closed"}
			return nil
		})
	})
	return result, err
}

func closeReasons() (string, string) {
	if i18n.Chinese() {
		return "因 Task 关闭而取消，未登记执行信息", "因 Task 关闭而终止，未完成"
	}
	return "cancelled because the Task closed before execution was registered", "cancelled because the Task closed before execution completed"
}

func (s *Service) ReopenTask(ctx context.Context, taskID *string) (Result, error) {
	if taskID != nil {
		if err := domain.ValidateID(*taskID, "task"); err != nil {
			return Result{}, err
		}
	}
	var result Result
	err := s.shared(ctx, func(ctx context.Context) error {
		return s.Store.WriteQ(ctx, func(q storesqlite.Queryer) error {
			var id, status string
			var runtime int
			if taskID != nil {
				err := q.QueryRowContext(ctx, `SELECT id,status FROM tasks WHERE id=?`, *taskID).Scan(&id, &status)
				if errors.Is(err, sql.ErrNoRows) {
					return domain.NewError("TASK_NOT_FOUND", "Task not found")
				}
				if err != nil {
					return dbErr(err)
				}
			} else {
				err := q.QueryRowContext(ctx, `SELECT id,status FROM tasks ORDER BY CASE status WHEN 'open' THEN 0 ELSE 1 END, created_at DESC, id ASC LIMIT 1`).Scan(&id, &status)
				if errors.Is(err, sql.ErrNoRows) {
					return domain.NewError("NO_TASK", "Workspace has no Task")
				}
				if err != nil {
					return dbErr(err)
				}
			}
			if err := q.QueryRowContext(ctx, `SELECT runtime_occupied FROM workspace_meta WHERE singleton=1`).Scan(&runtime); err != nil {
				return dbErr(err)
			}
			if status == "open" {
				if runtime != 1 {
					return domain.NewError("WORKSPACE_STATE_UNTRUSTED", "open Task is missing Runtime occupancy")
				}
				result = Result{Data: map[string]any{"id": id, "changed": false}, Message: "Task is already open"}
				return nil
			}
			var other int
			if err := q.QueryRowContext(ctx, `SELECT count(*) FROM tasks WHERE status='open'`).Scan(&other); err != nil {
				return dbErr(err)
			}
			if other != 0 {
				return domain.NewError("OPEN_TASK_EXISTS", "another Task is open")
			}
			if _, err := q.ExecContext(ctx, `UPDATE tasks SET status='open',outcome=NULL,outcome_reason=NULL,closed_at=NULL WHERE id=? AND status='closed'`, id); err != nil {
				return dbErr(err)
			}
			if _, err := q.ExecContext(ctx, `UPDATE workspace_meta SET runtime_occupied=1 WHERE singleton=1`); err != nil {
				return dbErr(err)
			}
			result = Result{Data: map[string]any{"id": id, "changed": true}, Message: "Task reopened"}
			return nil
		})
	})
	return result, err
}

type AddCognitionRequest struct {
	Title, Content, Origin string
	ExplorationID          *string
	Relevant               bool
}

func (s *Service) AddCognition(ctx context.Context, req AddCognitionRequest) (Result, error) {
	title, err := domain.NonEmpty(req.Title, "cognition-title")
	if err != nil {
		return Result{}, err
	}
	content, err := domain.NonEmpty(req.Content, "cognition-content")
	if err != nil {
		return Result{}, err
	}
	if req.Origin != "user" && req.Origin != "exploration" {
		return Result{}, domain.NewError("INVALID_ARGUMENT", "origin must be user or exploration")
	}
	if req.Origin == "exploration" && req.ExplorationID == nil {
		return Result{}, domain.NewError("INVALID_ARGUMENT", "exploration is required for origin=exploration")
	}
	if req.ExplorationID != nil {
		if err := domain.ValidateID(*req.ExplorationID, "exploration"); err != nil {
			return Result{}, err
		}
	}
	if req.Origin == "user" && req.ExplorationID == nil && req.Relevant {
		return Result{}, domain.NewError("INVALID_ARGUMENT", "global Cognition cannot be task-purpose-relevant")
	}
	var result Result
	err = s.shared(ctx, func(ctx context.Context) error {
		return s.Store.WriteQ(ctx, func(q storesqlite.Queryer) error {
			task, err := openTask(ctx, q)
			if err != nil {
				return err
			}
			id, e := s.IDs.CognitionID()
			if e != nil {
				return domain.Wrap("INTERNAL_ERROR", "cannot generate Cognition ID", e)
			}
			now := domain.TimeText(s.Clock.Now())
			var originRef string
			var taskID any
			var originCog, originExp any
			targetStatus := ""
			if req.ExplorationID == nil {
				originRef = task.InitCognitionRef
				originCog = originRef
				taskID = nil
			} else {
				originRef = *req.ExplorationID
				originExp = originRef
				taskID = task.ID
				var expTask string
				if e := q.QueryRowContext(ctx, `SELECT task_id,status FROM explorations WHERE id=?`, originRef).Scan(&expTask, &targetStatus); errors.Is(e, sql.ErrNoRows) {
					return domain.NewError("EXPLORATION_NOT_FOUND", "Exploration not found")
				} else if e != nil {
					return dbErr(e)
				}
				if req.Origin == "exploration" && (expTask != task.ID || (targetStatus != "todo" && targetStatus != "running")) {
					return domain.NewError("INVALID_STATE", "Explorer Cognition requires a todo/running Exploration in the open Task")
				}
			}
			if _, e := q.ExecContext(ctx, `INSERT INTO cognitions(id,title,content,task_id,origin,origin_cognition_id,origin_exploration_id,task_purpose_relevant,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, id, title, content, taskID, req.Origin, originCog, originExp, req.Relevant, now); e != nil {
				return entityInsertErr(e, "cognitions")
			}
			warnings, e := warnings(ctx, q)
			if e != nil {
				return e
			}
			data := map[string]any{"id": id, "origin": req.Origin, "origin_ref": originRef, "task_id": taskID, "created_at": now}
			if targetStatus != "" {
				data["exploration_status"] = targetStatus
			}
			result = Result{Data: data, Warnings: warnings, Message: "Cognition added"}
			return nil
		})
	})
	return result, err
}

type AddExplorationRequest struct {
	Title, Content string
	Sources        []string
}

func (s *Service) AddExploration(ctx context.Context, req AddExplorationRequest) (Result, error) {
	title, e := domain.NonEmpty(req.Title, "exploration-title")
	if e != nil {
		return Result{}, e
	}
	content, e := domain.NonEmpty(req.Content, "exploration-content")
	if e != nil {
		return Result{}, e
	}
	if e := uniqueIDs(req.Sources, "cognition"); e != nil {
		return Result{}, e
	}
	var result Result
	e = s.shared(ctx, func(ctx context.Context) error {
		return s.Store.WriteQ(ctx, func(q storesqlite.Queryer) error {
			task, e := openTask(ctx, q)
			if e != nil {
				return e
			}
			sources := append([]string(nil), req.Sources...)
			if len(sources) == 0 {
				sources = []string{task.InitCognitionRef}
			}
			if e := requireCognitions(ctx, q, sources); e != nil {
				return e
			}
			id, x := s.IDs.ExplorationID()
			if x != nil {
				return domain.Wrap("INTERNAL_ERROR", "cannot generate Exploration ID", x)
			}
			now := domain.TimeText(s.Clock.Now())
			if _, x := q.ExecContext(ctx, `INSERT INTO explorations(id,title,content,task_id,status,created_at) VALUES(?,?,?,?,'todo',?)`, id, title, content, task.ID, now); x != nil {
				return entityInsertErr(x, "explorations")
			}
			for i, c := range sources {
				if _, x := q.ExecContext(ctx, `INSERT INTO exploration_sources(exploration_id,cognition_id,ordinal) VALUES(?,?,?)`, id, c, i); x != nil {
					return dbErr(x)
				}
			}
			w, x := warnings(ctx, q)
			if x != nil {
				return x
			}
			result = Result{Data: map[string]any{"id": id, "title": title, "content": content, "source_cognitions": sources, "created_at": now}, Warnings: w, Message: "Exploration added"}
			return nil
		})
	})
	return result, e
}

func (s *Service) RetryExploration(ctx context.Context, source string) (Result, error) {
	if e := domain.ValidateID(source, "exploration"); e != nil {
		return Result{}, e
	}
	var result Result
	e := s.shared(ctx, func(ctx context.Context) error {
		return s.Store.WriteQ(ctx, func(q storesqlite.Queryer) error {
			task, e := openTask(ctx, q)
			if e != nil {
				return e
			}
			var title, content, status string
			if x := q.QueryRowContext(ctx, `SELECT title,content,status FROM explorations WHERE id=?`, source).Scan(&title, &content, &status); errors.Is(x, sql.ErrNoRows) {
				return domain.NewError("EXPLORATION_NOT_FOUND", "Exploration not found")
			} else if x != nil {
				return dbErr(x)
			}
			if status != "finished" {
				return domain.NewError("INVALID_STATE", "retry source must be finished")
			}
			sources, e := sourceIDs(ctx, q, source)
			if e != nil {
				return e
			}
			id, x := s.IDs.ExplorationID()
			if x != nil {
				return domain.Wrap("INTERNAL_ERROR", "cannot generate Exploration ID", x)
			}
			now := domain.TimeText(s.Clock.Now())
			if _, x := q.ExecContext(ctx, `INSERT INTO explorations(id,title,content,task_id,retry_of,status,created_at) VALUES(?,?,?,?,?,'todo',?)`, id, title, content, task.ID, source, now); x != nil {
				return entityInsertErr(x, "explorations")
			}
			for i, c := range sources {
				if _, x := q.ExecContext(ctx, `INSERT INTO exploration_sources VALUES(?,?,?)`, id, c, i); x != nil {
					return dbErr(x)
				}
			}
			w, x := warnings(ctx, q)
			if x != nil {
				return x
			}
			result = Result{Data: map[string]any{"id": id, "retry_of": source, "title": title, "content": content, "source_cognitions": sources, "created_at": now}, Warnings: w, Message: "Exploration retry created"}
			return nil
		})
	})
	return result, e
}

func (s *Service) AssignExploration(ctx context.Context, id, executor, outputRef string) (Result, error) {
	if e := domain.ValidateID(id, "exploration"); e != nil {
		return Result{}, e
	}
	executor, e := domain.NonEmpty(executor, "explorer")
	if e != nil {
		return Result{}, e
	}
	outputRef, e = domain.NonEmpty(outputRef, "output")
	if e != nil {
		return Result{}, e
	}
	outputRef = s.resolveOutputRef(outputRef)
	var result Result
	e = s.shared(ctx, func(ctx context.Context) error {
		return s.Store.WriteQ(ctx, func(q storesqlite.Queryer) error {
			task, e := openTask(ctx, q)
			if e != nil {
				return e
			}
			now := domain.TimeText(s.Clock.Now())
			r, x := q.ExecContext(ctx, `UPDATE explorations SET status='running',executor_id=?,output_ref=?,assigned_at=? WHERE id=? AND task_id=? AND status='todo'`, executor, outputRef, now, id, task.ID)
			if x != nil {
				return dbErr(x)
			}
			n, _ := r.RowsAffected()
			if n != 1 {
				return explorationStateError(ctx, q, id)
			}
			w, x := warnings(ctx, q)
			if x != nil {
				return x
			}
			result = Result{Data: map[string]any{"id": id, "status": "running", "executor_id": executor, "output_ref": outputRef, "assigned_at": now}, Warnings: w, Message: "Exploration assigned"}
			return nil
		})
	})
	return result, e
}

func (s *Service) resolveOutputRef(outputRef string) string {
	candidate := outputRef
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(s.Paths.Root, candidate)
	}
	info, err := os.Lstat(candidate)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return outputRef
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return outputRef
	}
	absolute, err := filepath.Abs(resolved)
	if err != nil {
		return outputRef
	}
	return filepath.Clean(absolute)
}

func (s *Service) FinishExploration(ctx context.Context, id, outcome, reason string) (Result, error) {
	if e := domain.ValidateID(id, "exploration"); e != nil {
		return Result{}, e
	}
	if !domain.ExplorationOutcomes[outcome] {
		return Result{}, domain.NewError("INVALID_ARGUMENT", "invalid Exploration outcome")
	}
	reason, e := domain.NonEmpty(reason, "outcome-reason")
	if e != nil {
		return Result{}, e
	}
	var result Result
	e = s.shared(ctx, func(ctx context.Context) error {
		return s.Store.WriteQ(ctx, func(q storesqlite.Queryer) error {
			task, e := openTask(ctx, q)
			if e != nil {
				return e
			}
			var status string
			if x := q.QueryRowContext(ctx, `SELECT status FROM explorations WHERE id=? AND task_id=?`, id, task.ID).Scan(&status); errors.Is(x, sql.ErrNoRows) {
				return explorationStateError(ctx, q, id)
			} else if x != nil {
				return dbErr(x)
			}
			if status == "finished" || (status == "todo" && outcome != "cancelled") {
				return domain.NewError("INVALID_STATE", "invalid Exploration finish transition")
			}
			now := domain.TimeText(s.Clock.Now())
			r, x := q.ExecContext(ctx, `UPDATE explorations SET status='finished',outcome=?,outcome_reason=?,finished_at=? WHERE id=? AND task_id=? AND status=?`, outcome, reason, now, id, task.ID, status)
			if x != nil {
				return dbErr(x)
			}
			n, _ := r.RowsAffected()
			if n != 1 {
				return domain.NewError("INVALID_STATE", "Exploration state changed concurrently")
			}
			w, x := warnings(ctx, q)
			if x != nil {
				return x
			}
			result = Result{Data: map[string]any{"id": id, "status": "finished", "outcome": outcome, "finished_at": now}, Warnings: w, Message: "Exploration finished"}
			return nil
		})
	})
	return result, e
}

type ExplorationList struct {
	TaskID         string           `json:"task_id"`
	MaxConcurrency int              `json:"max_concurrency"`
	Explorations   []map[string]any `json:"explorations"`
}

func (s *Service) ListExplorations(ctx context.Context, status string) (ExplorationList, error) {
	var out ExplorationList
	err := s.shared(ctx, func(ctx context.Context) error {
		return s.Store.Read(ctx, func(tx *sql.Tx) error {
			task, e := openTask(ctx, tx)
			if e != nil {
				return e
			}
			out.TaskID = task.ID
			out.MaxConcurrency = task.MaxConcurrency
			out.Explorations = []map[string]any{}
			rows, e := tx.QueryContext(ctx, `SELECT id,title,content,retry_of,executor_id,output_ref,assigned_at,created_at FROM explorations WHERE task_id=? AND status=? ORDER BY created_at ASC,id ASC`, task.ID, status)
			if e != nil {
				return dbErr(e)
			}
			defer rows.Close()
			now := s.Clock.Now()
			for rows.Next() {
				var id, title, content, created string
				var retry, executor, outputRef, assigned sql.NullString
				if e := rows.Scan(&id, &title, &content, &retry, &executor, &outputRef, &assigned, &created); e != nil {
					return dbErr(e)
				}
				sources, e := sourceIDs(ctx, tx, id)
				if e != nil {
					return e
				}
				item := map[string]any{"id": id, "title": title, "content": content, "source_cognitions": sources, "created_at": created}
				if retry.Valid {
					item["retry_of"] = retry.String
				}
				if status == "running" {
					item["executor_id"] = executor.String
					item["output_ref"] = outputRef.String
					item["assigned_at"] = assigned.String
					t, _ := time.Parse(time.RFC3339Nano, assigned.String)
					d := int64(now.Sub(t).Seconds())
					if d < 0 {
						d = 0
					}
					item["duration_seconds"] = d
				}
				out.Explorations = append(out.Explorations, item)
			}
			return rows.Err()
		})
	})
	return out, err
}

func openTask(ctx context.Context, q storesqlite.Queryer) (domain.Task, error) {
	var t domain.Task
	err := q.QueryRowContext(ctx, `SELECT id,purpose,init_cognition_ref,max_concurrency,status,created_at FROM tasks WHERE status='open'`).Scan(&t.ID, &t.Purpose, &t.InitCognitionRef, &t.MaxConcurrency, &t.Status, &t.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return t, domain.NewError("NO_OPEN_TASK", "Workspace has no open Task")
	}
	if err != nil {
		return t, dbErr(err)
	}
	return t, nil
}
func uniqueIDs(ids []string, prefix string) error {
	seen := map[string]bool{}
	for _, id := range ids {
		if e := domain.ValidateID(id, prefix); e != nil {
			return e
		}
		if seen[id] {
			return domain.NewError("INVALID_ARGUMENT", "duplicate source Cognition ID")
		}
		seen[id] = true
	}
	return nil
}
func requireCognitions(ctx context.Context, q storesqlite.Queryer, ids []string) error {
	for _, id := range ids {
		var one int
		if e := q.QueryRowContext(ctx, `SELECT 1 FROM cognitions WHERE id=?`, id).Scan(&one); errors.Is(e, sql.ErrNoRows) {
			return domain.NewError("COGNITION_NOT_FOUND", "Cognition not found: "+id)
		} else if e != nil {
			return dbErr(e)
		}
	}
	return nil
}
func sourceIDs(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, id string) ([]string, error) {
	rows, e := q.QueryContext(ctx, `SELECT cognition_id FROM exploration_sources WHERE exploration_id=? ORDER BY ordinal`, id)
	if e != nil {
		return nil, dbErr(e)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var v string
		if e := rows.Scan(&v); e != nil {
			return nil, dbErr(e)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func warnings(ctx context.Context, q storesqlite.Queryer) ([]output.Warning, error) {
	task, e := openTask(ctx, q)
	if e != nil {
		return nil, e
	}
	var count int
	if e := q.QueryRowContext(ctx, `SELECT count(*) FROM explorations WHERE task_id=? AND status='running'`, task.ID).Scan(&count); e != nil {
		return nil, dbErr(e)
	}
	if count > task.MaxConcurrency {
		return []output.Warning{output.ConcurrencyWarning(count, task.MaxConcurrency)}, nil
	}
	return []output.Warning{}, nil
}
func explorationStateError(ctx context.Context, q storesqlite.Queryer, id string) error {
	var taskID, status string
	e := q.QueryRowContext(ctx, `SELECT task_id,status FROM explorations WHERE id=?`, id).Scan(&taskID, &status)
	if errors.Is(e, sql.ErrNoRows) {
		return domain.NewError("EXPLORATION_NOT_FOUND", "Exploration not found")
	}
	if e != nil {
		return dbErr(e)
	}
	return domain.Errorf("INVALID_STATE", "Exploration %s is %s or does not belong to the open Task", id, status)
}
func noOpen(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return domain.NewError("NO_OPEN_TASK", "Workspace has no open Task")
	}
	return dbErr(err)
}
func dbErr(err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "locked") || strings.Contains(message, "busy") {
		typed := domain.Wrap("WORKSPACE_BUSY", "SQLite writer is busy", err)
		typed.Details["resource"] = "sqlite_write"
		return typed
	}
	if strings.Contains(message, "constraint") {
		return domain.Wrap("INVALID_STATE", "operation violates Workspace constraints", err)
	}
	return domain.Wrap("DATABASE_ERROR", "database operation failed", err)
}

func entityInsertErr(err error, table string) error {
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unique constraint failed: "+table+".id") || strings.Contains(message, "primary key") {
		return domain.Wrap("ID_CONFLICT", "generated entity ID already exists", err)
	}
	return dbErr(err)
}

// StableSortByCreation is shared by View builders that already loaded nodes.
func StableSortByCreation[T interface {
	GetCreatedAt() string
	GetID() string
}](items []T) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].GetCreatedAt() == items[j].GetCreatedAt() {
			return items[i].GetID() < items[j].GetID()
		}
		return items[i].GetCreatedAt() < items[j].GetCreatedAt()
	})
}
