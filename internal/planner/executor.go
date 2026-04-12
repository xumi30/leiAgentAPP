package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"leiAgent/internal/globalchannel"
	"leiAgent/internal/tools"
	"leiAgent/logging"
	"leiAgent/utils"
)

func (p *Planning) DoExe(ctx context.Context) (string, error) {
	var resultSteps string

	// 执行计划
	err := p.Execute(ctx)
	if err != nil {
		p.Status = utils.TaskFailed
		logging.Error("Execute failed: %v", err)
		return fmt.Sprintf("%v", p), err
	}

	for i := range p.Steps {
		if p.Steps[i].Result != nil {
			// 处理Result为字符串的情况
			if resultStr, ok := p.Steps[i].Result.(string); ok {
				var parsedResult interface{}
				if err := json.Unmarshal([]byte(resultStr), &parsedResult); err == nil {
					p.Steps[i].Result = parsedResult
					continue
				}
				logging.Error("解析结果为字符串失败，原字符串: %s", resultStr)

				p.Steps[i].Error = fmt.Sprintf("Result parse failed: %v", err)
			}

		}
	}

	// 序列化结果
	rst, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		logging.Error("Failed to marshal plan results: %v", err)
		return fmt.Sprintf("%v", p), err
	}
	resultSteps = string(rst)

	//logging.Info("Plan results: %s", resultSteps)
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

	//初始化所有步骤状态
	for i := range p.Steps {
		if p.Steps[i].Status != utils.StepCompleted {
			p.Steps[i].Status = utils.StepPending
		}
	}

	logging.Info("开始执行计划，共 %d 个步骤", len(p.Steps))
	globalchannel.SendAssitantMessageOnce(ctx, "开始执行计划，共 "+fmt.Sprint(len(p.Steps))+" 个步骤")

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
			globalchannel.SendAssitantMessageOnce(ctx, fmt.Sprintf("步骤 %s 执行失败: %v.跳过继续尝试执行剩余步骤...\n", p.Steps[current].Id, err))
			p.Status = utils.TaskFailed
			// return fmt.Errorf("step %s failed: %v", p.Steps[current].Id, err)
		}

		logging.Info("步骤 %s 执行完成, 结果是: %v", p.Steps[current].Id, p.Steps[current].Result)
		globalchannel.SendAssitantMessageOnce(ctx, fmt.Sprintf("步骤 %s 执行完成, 结果是: %v\n", p.Steps[current].Id, p.Steps[current].Result))

		// 使用反向依赖图更新依赖当前任务的其他任务的入度
		if dependentSteps, exists := reverseMap[p.Steps[current].Id]; exists {
			for _, depIndex := range dependentSteps {
				p.Steps[depIndex].InDegree--
				logging.Debug("更新步骤 %s 的依赖，剩余入度: %d", p.Steps[depIndex].Id, p.Steps[depIndex].InDegree)

				if p.Steps[depIndex].InDegree == 0 {
					queue = append(queue, depIndex)
					logging.Info("步骤 %s 所有依赖已完成，加入执行队列", p.Steps[depIndex].Id)

				}
			}
		}
	}

	// 检查是否所有任务都已完成
	logging.Info("检查所有步骤完成状态")
	for _, step := range p.Steps {
		if step.Status != utils.StepCompleted {
			p.Status = utils.TaskFailed
			logging.Error("步骤 %s 未完成，状态: %s", step.Id, step.Status)
		}
	}
	if p.Status == utils.TaskFailed {
		return fmt.Errorf("some steps failed")
	}

	logging.Info("所有步骤尝试执行完成")
	//logging.Info("计划执行成功,plan: %v", p)
	globalchannel.SendAssitantMessageOnce(ctx, "执行阶段小结：所有步骤尝试执行完成\n")
	return nil
}

func (p *Planning) ExecuteStep(ctx context.Context, stepIndex int) error {
	if p.Steps[stepIndex].Status == utils.StepCompleted {
		logging.Info("步骤 %s 已完成，跳过执行", p.Steps[stepIndex].Id)
		return nil
	}

	if stepIndex < 0 || stepIndex >= len(p.Steps) {
		return fmt.Errorf("invalid step index: %d", stepIndex)
	}

	step := &p.Steps[stepIndex]
	step.Status = utils.StepRunning

	// 获取工具
	toolRegistry := tools.Getregistry()
	tool, exists := toolRegistry.Get(step.Tool)
	if !exists {
		step.Status = utils.StepFailed
		step.Error = fmt.Sprintf("tool not found: %s", step.Tool)
		logging.Error("步骤 %s 执行失败: tool not found: %s", step.Id, step.Tool)
		return fmt.Errorf("tool not found: %s", step.Tool)
	}

	//处理输入引用
	processedInput := make(map[string]interface{})
	for key, iv := range step.Input {
		// 检查是否有直接输入值
		if iv.StepInputValue != nil && iv.StepInputValue != "" {
			processedInput[key] = iv.StepInputValue
			logging.Info("获取到直接输入值，key: %s, value: %v", key, iv.StepInputValue)
			continue
		}

		// 如果引用输入也为空，则跳过
		if iv.RefStepID == "" || iv.RefStepOutField == "" {
			logging.Info("步骤 %s 的输入引用为空，key: %s", step.Id, key)
			step.Status = utils.StepFailed
			step.Error = fmt.Sprintf("步骤 %s 的输入引用为空，key: %s", step.Id, key)
			continue
		}

		// 正确的写法：
		var refStep *Step
		for i := range p.Steps {
			if p.Steps[i].Id == iv.RefStepID {
				refStep = &p.Steps[i]
				break
			}
		}

		if refStep == nil {
			step.Status = utils.StepFailed
			step.Error = fmt.Sprintf("找不到引用的步骤 %s", iv.RefStepID)
			logging.Error("步骤 %s 执行失败: 找不到引用的步骤 %s", step.Id, iv.RefStepID)
			return fmt.Errorf("找不到引用的步骤 %s", iv.RefStepID)
		}

		// 检查依赖步骤是否已完成
		if refStep.Status != "completed" {
			step.Status = utils.StepFailed
			step.Error = fmt.Sprintf("依赖的步骤 %s 未完成", iv.RefStepID)
			logging.Error("步骤 %s 执行失败: 依赖的步骤 %s 未完成", step.Id, iv.RefStepID)
		}

		// 如果引用步骤的结果为空，则跳过处理
		if refStep.Result == nil {
			step.Status = utils.StepFailed
			step.Error = fmt.Sprintf("依赖的步骤 %s 的结果为空", iv.RefStepID)
			logging.Error("步骤 %s 执行失败: 依赖的步骤 %s 的结果为空", step.Id, iv.RefStepID)
		}

		// 解析引用步骤的结果
		var resultData map[string]interface{}
		// 将 string 转换为 []byte
		resultBytes, ok := refStep.Result.(string)
		if !ok {
			step.Status = utils.StepFailed
			step.Error = fmt.Sprintf("依赖的步骤 %s 的结果类型错误，期望 string", iv.RefStepID)
			logging.Error("步骤 %s 执行失败: 依赖的步骤 %s 的结果类型错误", step.Id, iv.RefStepID)
			return fmt.Errorf("依赖的步骤 %s 的结果类型错误", iv.RefStepID)
		}
		err := json.Unmarshal([]byte(resultBytes), &resultData)

		if err != nil {
			step.Status = utils.StepFailed
			step.Error = fmt.Sprintf("解析依赖的步骤 %s 的结果失败: %v", iv.RefStepID, err)
			logging.Error("步骤 %s 执行失败: 解析依赖的步骤 %s 的结果失败: %v", step.Id, iv.RefStepID, err)
		}
		// 取引用字段的值
		processedInput[key] = resultData[iv.RefStepOutField]

		// 更新步骤的输入值 下次重试时，可以直接使用processedInput作为输入值
		currentInput := step.Input[key]                              // 1. 取
		currentInput.StepInputValue = resultData[iv.RefStepOutField] // 2. 改
		step.Input[key] = currentInput                               // 3. 存
	}

	//将 processedInput 序列化为 JSON 字符串
	inputJSON, err := json.Marshal(processedInput)
	if err != nil {
		step.Status = utils.StepFailed
		step.Error = fmt.Sprintf("failed to marshal input: %v for step %s", err, step.Id)
		return fmt.Errorf("failed to marshal input: %v for step %s", err, step.Id)
	}

	result, err := tool.Execute(ctx, string(inputJSON))

	if err != nil {
		step.Status = utils.StepFailed
		step.Error = fmt.Sprintf("tool execution failed: %v", err)
		logging.Error("步骤 %s %s 执行失败: %v", step.Id, step.Tool, err)
		return fmt.Errorf("tool execution failed: %v", err)
	}
	logging.Info("步骤 %s 执行成功,结果: %v", p.Steps[stepIndex].Id, result)

	step.Result = result
	step.Status = utils.StepCompleted
	return nil
}
