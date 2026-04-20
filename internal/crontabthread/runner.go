package crontabthread

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"leiAgent/logging"
)

type Executor interface {
	Execute(ctx context.Context, task ScheduledTask) error
}

type DefaultExecutor struct{}

func (e *DefaultExecutor) Execute(ctx context.Context, task ScheduledTask) error {
	switch strings.TrimSpace(task.ActionType) {
	case "notify":
		logging.Info("[scheduled_task] notify: %s (task=%s)", strings.TrimSpace(task.ActionPayload), task.ID)
		return nil
	case "tool":
		// For now, just validate JSON payload; real tool dispatch can be wired later.
		var payload struct {
			Tool string                 `json:"tool"`
			Args map[string]interface{} `json:"args"`
		}
		if err := json.Unmarshal([]byte(task.ActionPayload), &payload); err != nil {
			return fmt.Errorf("tool action_payload must be JSON: %w", err)
		}
		if strings.TrimSpace(payload.Tool) == "" {
			return fmt.Errorf("tool action_payload missing tool")
		}
		logging.Info("[scheduled_task] tool: %s args=%v (task=%s)", payload.Tool, payload.Args, task.ID)
		return nil
	default:
		return fmt.Errorf("unknown action_type %q", task.ActionType)
	}
}

type Runner struct {
	pollInterval time.Duration
	limit        int
	executor     Executor
}

func NewRunner(pollInterval time.Duration, limit int, executor Executor) *Runner {
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}
	if limit <= 0 {
		limit = 100
	}
	if executor == nil {
		executor = &DefaultExecutor{}
	}
	return &Runner{
		pollInterval: pollInterval,
		limit:        limit,
		executor:     executor,
	}
}

func (r *Runner) Start(ctx context.Context) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	go r.loop(ctx, db)
	return nil
}

func (r *Runner) loop(ctx context.Context, db *sql.DB) {
	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.tick(ctx, db)
		}
	}
}

func (r *Runner) tick(ctx context.Context, db *sql.DB) {
	now := time.Now().UTC()
	rows, err := db.Query(`
		SELECT id, user_id, title, action_type, action_payload, schedule_type,
		       run_at, cron_expr, rrule, timezone, status, executing, version, completed,
		       run_count, last_run_at, next_run_at, created_at, updated_at
		FROM scheduled_task
		WHERE status = 'active' AND completed = 0 AND executing = 0
		  AND next_run_at IS NOT NULL AND next_run_at <= ?
		ORDER BY next_run_at ASC
		LIMIT ?`, now, r.limit)
	if err != nil {
		logging.Error("scheduled task query failed: %v", err)
		return
	}
	defer rows.Close()

	type rowTask struct {
		ScheduledTask
		RunAtDB     sql.NullTime
		CronExprDB  sql.NullString
		RRuleDB     sql.NullString
		LastRunDB   sql.NullTime
		NextRunDB   sql.NullTime
		CreatedAtDB sql.NullTime
		UpdatedAtDB sql.NullTime
		ExecutingDB int
		CompletedDB int
	}

	candidates := make([]rowTask, 0, r.limit)
	for rows.Next() {
		var t rowTask
		var executingInt int
		var completedInt int
		if err := rows.Scan(
			&t.ID, &t.UserID, &t.Title, &t.ActionType, &t.ActionPayload, &t.ScheduleType,
			&t.RunAtDB, &t.CronExprDB, &t.RRuleDB, &t.Timezone, &t.Status, &executingInt, &t.Version, &completedInt,
			&t.RunCount, &t.LastRunDB, &t.NextRunDB, &t.CreatedAtDB, &t.UpdatedAtDB,
		); err != nil {
			logging.Error("scheduled task scan failed: %v", err)
			return
		}
		t.Executing = executingInt != 0
		t.Completed = completedInt != 0
		if t.RunAtDB.Valid {
			t.RunAt = t.RunAtDB.Time
		}
		if t.CronExprDB.Valid {
			t.CronExpr = t.CronExprDB.String
		}
		if t.RRuleDB.Valid {
			t.RRule = t.RRuleDB.String
		}
		if t.LastRunDB.Valid {
			t.LastRunAt = t.LastRunDB.Time
		}
		if t.NextRunDB.Valid {
			t.NextRunAt = t.NextRunDB.Time
		}
		if t.CreatedAtDB.Valid {
			t.CreatedAt = t.CreatedAtDB.Time
		}
		if t.UpdatedAtDB.Valid {
			t.UpdatedAt = t.UpdatedAtDB.Time
		}
		candidates = append(candidates, t)
	}
	if err := rows.Err(); err != nil {
		logging.Error("scheduled task rows err: %v", err)
		return
	}

	for _, cand := range candidates {
		if err := r.tryExecuteOne(ctx, db, cand.ScheduledTask); err != nil {
			logging.Warn("scheduled task execute failed task=%s: %v", cand.ID, err)
		}
	}
}

func (r *Runner) tryExecuteOne(ctx context.Context, db *sql.DB, task ScheduledTask) error {
	// Optimistic lock + claim
	claimRes, err := db.Exec(
		`UPDATE scheduled_task
		 SET executing = 1, version = version + 1, updated_at = ?
		 WHERE id = ? AND status = 'active' AND completed = 0 AND executing = 0 AND version = ?`,
		time.Now().UTC(), task.ID, task.Version,
	)
	if err != nil {
		return fmt.Errorf("claim: %w", err)
	}
	ra, _ := claimRes.RowsAffected()
	if ra == 0 {
		return nil // lost race
	}

	execNow := time.Now().UTC()
	execErr := r.executor.Execute(ctx, task)

	// Compute next run
	loc, tz, err := loadLocation(task.Timezone)
	if err != nil {
		tz = "Local"
		loc = time.Local
	}

	var next time.Time
	var completed int
	var nextDB interface{}

	if strings.TrimSpace(task.ScheduleType) == "once" {
		// Once: mark completed regardless of execution error to prevent repeated spam;
		// you can change policy later if you want retries.
		completed = 1
		nextDB = nil
	} else {
		next, err = computeNextRun(execNow, task.ScheduleType, task.RunAt, task.CronExpr, task.RRule, loc)
		if err != nil {
			// If cannot compute next run, pause the task to avoid hot looping.
			_, _ = db.Exec(`UPDATE scheduled_task SET status='paused' WHERE id=?`, task.ID)
			nextDB = nil
		} else {
			nextDB = next.UTC()
		}
	}

	status := task.Status
	if completed == 1 {
		status = "paused"
	}

	_, updErr := db.Exec(
		`UPDATE scheduled_task
		 SET executing = 0,
		     status = ?,
		     completed = ?,
		     run_count = run_count + 1,
		     last_run_at = ?,
		     next_run_at = ?,
		     updated_at = ?
		 WHERE id = ?`,
		status,
		completed,
		execNow,
		nextDB,
		time.Now().UTC(),
		task.ID,
	)
	if updErr != nil {
		return fmt.Errorf("update after exec: %w", updErr)
	}

	if execErr != nil {
		return execErr
	}
	_ = tz
	return nil
}

