package crontabthread

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"leiAgent/logging"
)

type ListOptions struct {
	Status         string
	IncludeDeleted bool
	Limit          int
	Offset         int
}

func ListTasks(opts ListOptions) ([]map[string]interface{}, error) {
	start := time.Now()
	logging.Info("[scheduled_task:list] request status=%q includeDeleted=%v limit=%d offset=%d", strings.TrimSpace(opts.Status), opts.IncludeDeleted, opts.Limit, opts.Offset)

	db, err := openDB()
	if err != nil {
		logging.Error("[scheduled_task:list] openDB failed: %v", err)
		return nil, err
	}
	limit := opts.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset := opts.Offset
	if offset < 0 {
		offset = 0
	}

	where := make([]string, 0, 4)
	args := make([]interface{}, 0, 6)

	status := strings.TrimSpace(opts.Status)
	if status != "" {
		where = append(where, "status = ?")
		args = append(args, status)
	} else if !opts.IncludeDeleted {
		where = append(where, "status != 'deleted'")
	}

	query := `
		SELECT id, user_id, title, action_type, action_payload, schedule_type,
		       run_at, cron_expr, rrule, timezone, status,
		       executing, version, completed, run_count,
		       last_run_at, next_run_at, created_at, updated_at
		FROM scheduled_task`
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	logging.Info("[scheduled_task:list] sql=%s args=%v", strings.Join(strings.Fields(query), " "), args)
	rows, err := db.Query(query, args...)
	if err != nil {
		logging.Error("[scheduled_task:list] query failed: %v", err)
		return nil, fmt.Errorf("list scheduled_task: %w", err)
	}
	defer rows.Close()

	out := make([]map[string]interface{}, 0)
	for rows.Next() {
		var (
			id, userID, title, actionType, actionPayload, scheduleType, timezone, status string
			runAt                                                                        sql.NullTime
			cronExpr                                                                     sql.NullString
			rruleText                                                                    sql.NullString
			executingInt                                                                 int
			version                                                                      int
			completedInt                                                                 int
			runCount                                                                     int
			lastRunAt                                                                    sql.NullTime
			nextRunAt                                                                    sql.NullTime
			createdAt                                                                    time.Time
			updatedAt                                                                    time.Time
		)
		if err := rows.Scan(
			&id, &userID, &title, &actionType, &actionPayload, &scheduleType,
			&runAt, &cronExpr, &rruleText, &timezone, &status,
			&executingInt, &version, &completedInt, &runCount,
			&lastRunAt, &nextRunAt, &createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan scheduled_task: %w", err)
		}
		item := map[string]interface{}{
			"id":             id,
			"user_id":        userID,
			"title":          title,
			"action_type":    actionType,
			"action_payload": actionPayload,
			"schedule_type":  scheduleType,
			"timezone":       timezone,
			"status":         status,
			"executing":      executingInt != 0,
			"version":        version,
			"completed":      completedInt != 0,
			"run_count":      runCount,
			"created_at":     createdAt,
			"updated_at":     updatedAt,
		}
		if runAt.Valid {
			item["run_at"] = runAt.Time
		}
		if cronExpr.Valid {
			item["cron_expr"] = cronExpr.String
		}
		if rruleText.Valid {
			item["rrule"] = rruleText.String
		}
		if lastRunAt.Valid {
			item["last_run_at"] = lastRunAt.Time
		}
		if nextRunAt.Valid {
			item["next_run_at"] = nextRunAt.Time
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		logging.Error("[scheduled_task:list] rows err: %v", err)
		return nil, err
	}
	logging.Info("[scheduled_task:list] ok count=%d elapsed=%s", len(out), time.Since(start))
	return out, nil
}

func SetTaskStatus(id string, status string) error {
	taskID := strings.TrimSpace(id)
	if taskID == "" {
		return fmt.Errorf("id is empty")
	}
	s := strings.ToLower(strings.TrimSpace(status))
	if s != "active" && s != "paused" && s != "deleted" {
		return fmt.Errorf("invalid status: %s", status)
	}
	db, err := openDB()
	if err != nil {
		return err
	}
	now := time.Now()
	res, err := db.Exec(`UPDATE scheduled_task SET status = ?, updated_at = ? WHERE id = ?`, s, now, taskID)
	if err != nil {
		return fmt.Errorf("update scheduled_task status: %w", err)
	}
	aff, _ := res.RowsAffected()
	logging.Info("[scheduled_task:set_status] id=%s status=%s affected=%d", taskID, s, aff)
	return nil
}

func DeleteTask(id string) error {
	return SetTaskStatus(id, "deleted")
}

func UpdateTaskBasics(id string, title string, actionPayload string) error {
	taskID := strings.TrimSpace(id)
	if taskID == "" {
		return fmt.Errorf("id is empty")
	}
	t := strings.TrimSpace(title)
	if t == "" {
		return fmt.Errorf("title is empty")
	}
	ap := strings.TrimSpace(actionPayload)
	db, err := openDB()
	if err != nil {
		return err
	}
	now := time.Now()
	res, err := db.Exec(`UPDATE scheduled_task SET title = ?, action_payload = ?, updated_at = ? WHERE id = ?`, t, ap, now, taskID)
	if err != nil {
		return fmt.Errorf("update scheduled_task basics: %w", err)
	}
	aff, _ := res.RowsAffected()
	logging.Info("[scheduled_task:update_basics] id=%s affected=%d", taskID, aff)
	return nil
}
