package dataoperation

import (
	"errors"
	"leiAgent/logging"
)

var (
	ErrSQLInstanceNotAvailable = errors.New("SQL实例不可用")
	ErrPlanNotFound            = errors.New("计划不存在")
)

// GetPlanByID 根据计划ID获取单个计划
func GetPlanByID(planID int64) (map[string]interface{}, error) {
	sql := GetSqlInstance()
	if sql == nil {
		logging.Error("获取SQL实例失败")
		return nil, ErrSQLInstanceNotAvailable
	}

	plan, err := sql.GetPlan(planID)
	if err != nil {
		if err.Error() == "plan not found" {
			logging.Info("未找到ID为 %d 的计划", planID)
			return nil, ErrPlanNotFound
		}
		logging.Error("获取计划失败: %v", err)
		return nil, err
	}

	logging.Info("成功获取计划ID %d", planID)
	return plan, nil
}

// ListPlans 列出所有计划
func ListPlans() ([]map[string]interface{}, error) {
	sql := GetSqlInstance()
	if sql == nil {
		logging.Error("获取SQL实例失败")
		return nil, ErrSQLInstanceNotAvailable
	}

	plans, err := sql.ListPlans()
	if err != nil {
		logging.Error("获取计划列表失败: %v", err)
		return nil, err
	}

	logging.Info("成功获取 %d 个计划", len(plans))
	return plans, nil
}

// SavePlan 保存计划
func SavePlan(goal string, status string, retryCount int) error {
	sql := GetSqlInstance()
	if sql == nil {
		logging.Error("获取SQL实例失败")
		return ErrSQLInstanceNotAvailable
	}

	err := sql.SavePlan(goal, status, retryCount)
	if err != nil {
		logging.Error("保存计划失败: %v", err)
		return err
	}

	logging.Info("计划保存成功 - 目标: %s, 状态: %s", goal, status)
	return nil
}

// DeletePlan 删除计划
func DeletePlan(planID int64) error {
	sql := GetSqlInstance()
	if sql == nil {
		logging.Error("获取SQL实例失败")
		return ErrSQLInstanceNotAvailable
	}

	err := sql.DeletePlan(planID)
	if err != nil {
		logging.Error("删除计划失败: %v", err)
		return err
	}

	logging.Info("成功删除计划ID %d", planID)
	return nil
}

// UpdatePlanStatus 更新计划状态
func UpdatePlanStatus(planID int64, status string) error {
	sql := GetSqlInstance()
	if sql == nil {
		logging.Error("获取SQL实例失败")
		return ErrSQLInstanceNotAvailable
	}

	err := sql.UpdatePlanStatus(planID, status)
	if err != nil {
		logging.Error("更新计划状态失败: %v", err)
		return err
	}

	logging.Info("成功更新计划ID %d 状态为: %s", planID, status)
	return nil
}

// PlanStep operations

// SavePlanStep 保存计划步骤
func SavePlanStep(planID int64, stepID, tool, input, dependsOn string, result interface{}, status, errorMsg string, indegree int) error {
	sql := GetSqlInstance()
	if sql == nil {
		logging.Error("获取SQL实例失败")
		return ErrSQLInstanceNotAvailable
	}

	err := sql.SavePlanStep(planID, stepID, tool, input, dependsOn, result, status, errorMsg, indegree)
	if err != nil {
		logging.Error("保存计划步骤失败: %v", err)
		return err
	}

	logging.Info("成功保存计划 %d 步骤 %s", planID, stepID)
	return nil
}

// GetPlanStep 获取计划步骤
func GetPlanStep(planID int64, stepID string) (map[string]interface{}, error) {
	sql := GetSqlInstance()
	if sql == nil {
		logging.Error("获取SQL实例失败")
		return nil, ErrSQLInstanceNotAvailable
	}

	step, err := sql.GetPlanStep(planID, stepID)
	if err != nil {
		logging.Error("获取计划步骤失败: %v", err)
		return nil, err
	}

	return step, nil
}

// ListPlanSteps 列出计划的所有步骤
func ListPlanSteps(planID int64) ([]map[string]interface{}, error) {
	sql := GetSqlInstance()
	if sql == nil {
		logging.Error("获取SQL实例失败")
		return nil, ErrSQLInstanceNotAvailable
	}

	steps, err := sql.ListPlanSteps(planID)
	if err != nil {
		logging.Error("获取计划步骤列表失败: %v", err)
		return nil, err
	}

	logging.Info("成功获取计划 %d 的 %d 个步骤", planID, len(steps))
	return steps, nil
}

// UpdatePlanStepStatus 更新计划步骤状态
func UpdatePlanStepStatus(planID int64, stepID, status string) error {
	sql := GetSqlInstance()
	if sql == nil {
		logging.Error("获取SQL实例失败")
		return ErrSQLInstanceNotAvailable
	}

	err := sql.UpdatePlanStepStatus(planID, stepID, status)
	if err != nil {
		logging.Error("更新计划步骤状态失败: %v", err)
		return err
	}

	logging.Info("成功更新计划 %d 步骤 %s 状态为: %s", planID, stepID, status)
	return nil
}

// UpdatePlanStepResult 更新计划步骤结果
func UpdatePlanStepResult(planID int64, stepID string, result interface{}, errorMsg string) error {
	sql := GetSqlInstance()
	if sql == nil {
		logging.Error("获取SQL实例失败")
		return ErrSQLInstanceNotAvailable
	}

	err := sql.UpdatePlanStepResult(planID, stepID, result, errorMsg)
	if err != nil {
		logging.Error("更新计划步骤结果失败: %v", err)
		return err
	}

	logging.Info("成功更新计划 %d 步骤 %s 结果", planID, stepID)
	return nil
}
