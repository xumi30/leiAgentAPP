package sqlmemory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"leiAgent/logging"
	"time"
)

const planstable = `CREATE TABLE IF NOT EXISTS plans (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    goal TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending', 'running', 'completed', 'failed')),
    retry_count INTEGER NOT NULL DEFAULT 6,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
)`

const planstepstable = `CREATE TABLE IF NOT EXISTS plan_steps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    plan_id INTEGER NOT NULL,
    step_id TEXT NOT NULL,
    tool TEXT NOT NULL,
    input TEXT,
    depends_on TEXT,
    result TEXT,
    status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending', 'running', 'completed', 'failed')),
    error TEXT,
    indegree INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(plan_id, step_id),
    FOREIGN KEY (plan_id) REFERENCES plans(id) ON DELETE CASCADE
)`

// SavePlan 保存或更新计划
func (m *SQLMemory) SavePlan(goal string, status string, retryCount int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	query := `
        INSERT INTO plans (id, goal, status, retry_count, updated_at)
        VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
        ON CONFLICT(id) DO UPDATE SET
            goal = excluded.goal,
            status = excluded.status,
            retry_count = excluded.retry_count,
            updated_at = CURRENT_TIMESTAMP
    `

	_, err := m.db.Exec(query, goal, status, retryCount)
	if err != nil {
		return fmt.Errorf("failed to save plan: %w", err)
	}

	return nil
}

// GetPlan 获取计划信息
func (m *SQLMemory) GetPlan(planID int64) (map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `SELECT id, goal, status, retry_count, created_at, updated_at FROM plans WHERE id = ?`

	row := m.db.QueryRow(query, planID)

	var id int64
	var goal, status string
	var retryCount int
	var createdAt, updatedAt time.Time

	if err := row.Scan(&id, &goal, &status, &retryCount, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			logging.Warn("Plan with ID %d not found", planID)
			return nil, fmt.Errorf("plan not found")
		}
		return nil, fmt.Errorf("failed to get plan: %w", err)
	}

	return map[string]interface{}{
		"id":          id,
		"goal":        goal,
		"status":      status,
		"retry_count": retryCount,
		"created_at":  createdAt,
		"updated_at":  updatedAt,
	}, nil
}

// ListPlans 列出所有计划
func (m *SQLMemory) ListPlans() ([]map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `SELECT id, goal, status, retry_count, created_at, updated_at FROM plans ORDER BY created_at DESC`

	rows, err := m.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list plans: %w", err)
	}
	defer rows.Close()

	var plans []map[string]interface{}

	for rows.Next() {
		var id int64
		var goal, status string
		var retryCount int
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &goal, &status, &retryCount, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan plan: %w", err)
		}

		plans = append(plans, map[string]interface{}{
			"id":          id,
			"goal":        goal,
			"status":      status,
			"retry_count": retryCount,
			"created_at":  createdAt,
			"updated_at":  updatedAt,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating plans: %w", err)
	}

	return plans, nil
}

// DeletePlan 删除计划及其所有步骤
func (m *SQLMemory) DeletePlan(planID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// SQLite 的外键约束会自动删除相关步骤
	_, err := m.db.Exec("DELETE FROM plans WHERE id = ?", planID)
	if err != nil {
		return fmt.Errorf("failed to delete plan: %w", err)
	}

	return nil
}

// UpdatePlanStatus 更新计划状态
func (m *SQLMemory) UpdatePlanStatus(planID int64, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	query := `
        UPDATE plans 
        SET status = ?, updated_at = CURRENT_TIMESTAMP 
        WHERE id = ?
    `

	_, err := m.db.Exec(query, status, planID)
	if err != nil {
		return fmt.Errorf("failed to update plan status: %w", err)
	}

	return nil
}

// SavePlanStep 保存或更新计划步骤
func (m *SQLMemory) SavePlanStep(planID int64, stepID, tool, input, dependsOn string, result interface{}, status, error string, indegree int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 将结果转换为JSON字符串
	var resultStr string
	if result != nil {
		if bytes, err := json.Marshal(result); err == nil {
			resultStr = string(bytes)
		}
	}

	query := `
        INSERT INTO plan_steps (plan_id, step_id, tool, input, depends_on, result, status, error, indegree, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
        ON CONFLICT(plan_id, step_id) DO UPDATE SET
            tool = excluded.tool,
            input = excluded.input,
            depends_on = excluded.depends_on,
            result = excluded.result,
            status = excluded.status,
            error = excluded.error,
            indegree = excluded.indegree,
            updated_at = CURRENT_TIMESTAMP
    `

	_, err := m.db.Exec(query, planID, stepID, tool, input, dependsOn, resultStr, status, error, indegree)
	if err != nil {
		return fmt.Errorf("failed to save plan step: %w", err)
	}

	return nil
}

// GetPlanStep 获取计划步骤
func (m *SQLMemory) GetPlanStep(planID int64, stepID string) (map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `SELECT id, plan_id, step_id, tool, input, depends_on, result, status, error, indegree, created_at, updated_at FROM plan_steps WHERE plan_id = ? AND step_id = ?`

	row := m.db.QueryRow(query, planID, stepID)

	var id, pID int64
	var sID, tool, input, dependsOn, resultStr, status, errorMsg string
	var indegree int
	var createdAt, updatedAt time.Time

	if err := row.Scan(&id, &pID, &sID, &tool, &input, &dependsOn, &resultStr, &status, &errorMsg, &indegree, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			logging.Warn("Plan step with plan ID %d and step ID %s not found", planID, stepID)
			return nil, fmt.Errorf("plan step not found")
		}
		return nil, fmt.Errorf("failed to get plan step: %w", err)
	}

	// 将结果字符串转换回原始类型
	var result interface{}
	if resultStr != "" {
		if err := json.Unmarshal([]byte(resultStr), &result); err != nil {
			logging.Warn("Failed to unmarshal result: %v", err)
		}
	}

	return map[string]interface{}{
		"id":         id,
		"plan_id":    pID,
		"step_id":    sID,
		"tool":       tool,
		"input":      input,
		"depends_on": dependsOn,
		"result":     result,
		"status":     status,
		"error":      errorMsg,
		"indegree":   indegree,
		"created_at": createdAt,
		"updated_at": updatedAt,
	}, nil
}

// ListPlanSteps 列出计划的所有步骤
func (m *SQLMemory) ListPlanSteps(planID int64) ([]map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `SELECT id, plan_id, step_id, tool, input, depends_on, result, status, error, indegree, created_at, updated_at FROM plan_steps WHERE plan_id = ? ORDER BY created_at`

	rows, err := m.db.Query(query, planID)
	if err != nil {
		return nil, fmt.Errorf("failed to list plan steps: %w", err)
	}
	defer rows.Close()

	var steps []map[string]interface{}

	for rows.Next() {
		var id, pID int64
		var sID, tool, input, dependsOn, resultStr, status, errorMsg string
		var indegree int
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &pID, &sID, &tool, &input, &dependsOn, &resultStr, &status, &errorMsg, &indegree, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan plan step: %w", err)
		}

		// 将结果字符串转换回原始类型
		var result interface{}
		if resultStr != "" {
			if err := json.Unmarshal([]byte(resultStr), &result); err != nil {
				logging.Warn("Failed to unmarshal result: %v", err)
			}
		}

		steps = append(steps, map[string]interface{}{
			"id":         id,
			"plan_id":    pID,
			"step_id":    sID,
			"tool":       tool,
			"input":      input,
			"depends_on": dependsOn,
			"result":     result,
			"status":     status,
			"error":      errorMsg,
			"indegree":   indegree,
			"created_at": createdAt,
			"updated_at": updatedAt,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating plan steps: %w", err)
	}

	return steps, nil
}

// UpdatePlanStepStatus 更新计划步骤状态
func (m *SQLMemory) UpdatePlanStepStatus(planID int64, stepID, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	query := `
        UPDATE plan_steps 
        SET status = ?, updated_at = CURRENT_TIMESTAMP 
        WHERE plan_id = ? AND step_id = ?
    `

	_, err := m.db.Exec(query, status, planID, stepID)
	if err != nil {
		return fmt.Errorf("failed to update plan step status: %w", err)
	}

	return nil
}

// UpdatePlanStepResult 更新计划步骤结果
func (m *SQLMemory) UpdatePlanStepResult(planID int64, stepID string, result interface{}, errorMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 将结果转换为JSON字符串
	var resultStr string
	if result != nil {
		if bytes, err := json.Marshal(result); err == nil {
			resultStr = string(bytes)
		}
	}

	query := `
        UPDATE plan_steps 
        SET result = ?, error = ?, updated_at = CURRENT_TIMESTAMP 
        WHERE plan_id = ? AND step_id = ?
    `

	_, err := m.db.Exec(query, resultStr, errorMsg, planID, stepID)
	if err != nil {
		return fmt.Errorf("failed to update plan step result: %w", err)
	}

	return nil
}
