package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"leiAgent/internal/globalchannel"
	"leiAgent/internal/memory"
	"leiAgent/internal/tools"
	"leiAgent/logging"
	"leiAgent/utils"
	"strings"
)

func (p *Planning) DoExe(ctx context.Context) (string, error) {
	var resultSteps string

	// 执行计划
	err := p.Execute(ctx)
	if err != nil {
		logging.Error("Execute failed: %v", err)
		p.Status = "failed"
		// 即使执行失败，也继续处理已成功步骤的结果
	}

	// 预处理所有Result字段
	for i := range p.Steps {
		if p.Steps[i].Result != nil {
			// 处理Result为字符串的情况
			if resultStr, ok := p.Steps[i].Result.(string); ok {
				var parsedResult interface{}
				if err := json.Unmarshal([]byte(resultStr), &parsedResult); err == nil {
					p.Steps[i].Result = parsedResult
				}
				// 如果解析失败，保留原字符串
			}
			// 处理Result已经是map的情况，不需要转换
		}
	}

	// 序列化结果
	rst, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		logging.Error("Failed to marshal plan results: %v", err)
		return "", err
	}
	resultSteps = string(rst)

	logging.Info("Plan results: %s", resultSteps)
	return resultSteps, nil
}

func (p *Planning) initializeInDegrees() {
	// 初始化每个步骤的入度
	for i := range p.Steps {
		p.Steps[i].InDegree = len(p.Steps[i].DependsOn)
	}
}

func (p *Planning) buildReverseDependencyMap() map[string][]int {
	// 构建反向依赖图：key是步骤ID，value是依赖该步骤的步骤索引列表
	reverseMap := make(map[string][]int)
	for i, step := range p.Steps {
		for _, depID := range step.DependsOn {
			reverseMap[depID] = append(reverseMap[depID], i)
		}
	}
	return reverseMap
}

func (p *Planning) Execute(ctx context.Context) error {

	chatID, ok := ctx.Value(utils.ChatIDString).(string)
	if !ok {
		logging.Error("Failed to get chatID from context")
		return fmt.Errorf("Failed to get chatID from context")
	}
	dialogOutChan := globalchannel.GetGlobalDialogOutChannel(chatID)
	//初始化所有步骤状态
	for i := range p.Steps {
		if p.Steps[i].Status != "completed" {
			p.Steps[i].Status = "pending"
		}
	}

	logging.Info("开始执行计划，共 %d 个步骤", len(p.Steps))

	// 初始化入度
	p.initializeInDegrees()
	logging.Info("初始化步骤入度完成")

	// 构建反向依赖图
	reverseMap := p.buildReverseDependencyMap()
	logging.Info("构建反向依赖图完成")

	// 使用队列存储入度为0的任务
	queue := make([]int, 0)
	for i, step := range p.Steps {
		if step.InDegree == 0 {
			queue = append(queue, i)
		}
	}
	logging.Info("找到 %d 个无依赖的初始步骤", len(queue))

	// 执行任务
	for len(queue) > 0 {
		// 从队列中取出一个任务
		current := queue[0]
		queue = queue[1:]

		logging.Info("开始执行步骤 %s (工具: %s)", p.Steps[current].Id, p.Steps[current].Tool)

		// 执行当前任务
		if err := p.ExecuteStep(ctx, current); err != nil {
			logging.Error("步骤 %s 执行失败: %v", p.Steps[current].Id, err)
			dialogOutChan <- &globalchannel.Message{Content: fmt.Sprintf("步骤 %s 执行失败: %v.跳过继续尝试执行剩余步骤...\n", p.Steps[current].Id, err), Role: utils.MessageRoleAssistant, IsFinished: false}
			memory.AddUserMessage(chatID, fmt.Sprintf("步骤 %s 执行失败: %v.跳过继续尝试执行剩余步骤...\n", p.Steps[current].Id, err))
			// return fmt.Errorf("step %s failed: %v", p.Steps[current].Id, err)
		}

		logging.Info("步骤 %s 执行完成, 结果是: %v", p.Steps[current].Id, p.Steps[current].Result)
		dialogOutChan <- &globalchannel.Message{Content: fmt.Sprintf("步骤 %s %s 执行完成, 结果是: %v\n", p.Steps[current].Id, p.Steps[current].Tool, p.Steps[current].Result), Role: utils.MessageRoleAssistant, IsFinished: false}
		memory.AddUserMessage(chatID, fmt.Sprintf("步骤 %s 执行完成, 结果是: %v\n", p.Steps[current].Id, p.Steps[current].Result))
		// 使用反向依赖图更新依赖当前任务的其他任务的入度
		if dependentSteps, exists := reverseMap[p.Steps[current].Id]; exists {
			for _, depIndex := range dependentSteps {
				p.Steps[depIndex].InDegree--
				logging.Debug("更新步骤 %s 的依赖，剩余入度: %d", p.Steps[depIndex].Id, p.Steps[depIndex].InDegree)
				if p.Steps[depIndex].InDegree == 0 {
					queue = append(queue, depIndex)
					logging.Info("步骤 %s 所有依赖已完成，加入执行队列", p.Steps[depIndex].Id)
					dialogOutChan <- &globalchannel.Message{Content: fmt.Sprintf("步骤 %s %s 所有依赖已完成，加入执行队列\n", p.Steps[depIndex].Id, p.Steps[depIndex].Tool), Role: utils.MessageRoleAssistant, IsFinished: false}
				}
			}
		}
	}

	// 检查是否所有任务都已完成
	logging.Info("检查所有步骤完成状态")
	for _, step := range p.Steps {
		if step.Status != "completed" {
			p.Status = "failed"
			logging.Error("步骤 %s 未完成，状态: %s", step.Id, step.Status)
			dialogOutChan <- &globalchannel.Message{Content: utils.FinishString, Role: utils.MessageRoleAssistant, IsFinished: true}
			dialogOutChan <- &globalchannel.Message{Content: fmt.Sprintf("执行阶段小结：步骤 %s %s 未完成，状态: %s", step.Id, step.Tool, step.Status), Role: utils.MessageRoleAssistant, IsFinished: false}
			//return fmt.Errorf("some Steps are not completed, possibly due to circular dependencies")
		}
	}

	logging.Info("所有步骤尝试执行完成")
	//logging.Info("计划执行成功,plan: %v", p)
	dialogOutChan <- &globalchannel.Message{Content: utils.FinishString, Role: utils.MessageRoleAssistant, IsFinished: true}
	return nil
}

func (p *Planning) ExecuteStep(ctx context.Context, stepIndex int) error {
	if p.Steps[stepIndex].Status == "completed" {
		logging.Info("步骤 %s 已完成，跳过执行", p.Steps[stepIndex].Id)
		return nil
	}

	if stepIndex < 0 || stepIndex >= len(p.Steps) {
		return fmt.Errorf("invalid step index: %d", stepIndex)
	}

	step := &p.Steps[stepIndex]
	step.Status = "running"

	// 获取工具
	toolRegistry := tools.Getregistry()
	tool, exists := toolRegistry.Get(step.Tool)
	if !exists {
		step.Status = "failed"
		step.Error = fmt.Sprintf("tool not found: %s", step.Tool)
		logging.Error("步骤 %s 执行失败: tool not found: %s", step.Id, step.Tool)
		return fmt.Errorf("tool not found: %s", step.Tool)
	}

	// 处理输入引用
	processedInput := make(map[string]interface{})
	for key, value := range step.Input {
		if refMap, ok := value.(map[string]interface{}); ok {
			if ref, ok := refMap["ref"].(string); ok {
				// 解析引用，如 "step_1.output.latitude"
				parts := strings.Split(ref, ".")
				if len(parts) >= 3 {
					refStepID := parts[0]
					refField := parts[2] // 提取字段名，如 "latitude" 或 "longitude"

					// 查找依赖步骤
					for i, s := range p.Steps {
						if s.Id == refStepID && i < stepIndex {
							// 从步骤输出中提取值
							if s.Result != nil {
								// 尝试解析结果为 JSON
								var resultData map[string]interface{}
								if resultStr, ok := s.Result.(string); ok {
									if err := json.Unmarshal([]byte(resultStr), &resultData); err == nil {
										// 从解析后的数据中提取字段值
										if fieldValue, ok := resultData[refField]; ok {
											processedInput[key] = fieldValue
										}
									}
								} else if resultMap, ok := s.Result.(map[string]interface{}); ok {
									// 如果已经是 map，直接提取字段值
									if fieldValue, ok := resultMap[refField]; ok {
										processedInput[key] = fieldValue
									}
								}
							}
						}
					}
				}
			}
		} else {
			processedInput[key] = value
		}
	}

	// 将 processedInput 序列化为 JSON 字符串
	inputJSON, err := json.Marshal(processedInput)
	if err != nil {
		step.Status = "failed"
		step.Error = fmt.Sprintf("failed to marshal input: %v", err)
		return fmt.Errorf("failed to marshal input: %v", err)
	}

	// 执行工具
	result, err := tool.Execute(ctx, string(inputJSON))

	if err != nil {
		step.Status = "failed"
		step.Error = fmt.Sprintf("tool execution failed: %v", err)
		logging.Error("步骤 %s %s 执行失败: %v", step.Id, step.Tool, err)
		return fmt.Errorf("tool execution failed: %v", err)
	}
	logging.Info("步骤 %s 执行成功,结果: %v", p.Steps[stepIndex].Id, result)

	step.Result = result
	step.Status = "completed"
	return nil
}
