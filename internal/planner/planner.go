package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"leiAgent/internal/memory"
	"leiAgent/internal/tools"
	"leiAgent/logging"
	"leiAgent/utils"
	"strings"
)

const (
	PlannerPromotion = `You are a task planner for an execution engine.

Your job is to convert a user goal into a structured execution plan.

The plan MUST be a valid JSON object following the exact schema below.

You MUST NOT execute any tools.
You MUST NOT explain anything.
You MUST ONLY output JSON.

----------------------------------------
PLAN FORMAT:

{
  "Steps": [
    {
      "id": "string",
      "tool": "string",
      "depends_on": ["string"],
      "input": { }
    }
  ]
}

----------------------------------------
RULES:
Fist of all: MUST output JSON following the exact schema above, or it will be rejected。Start with the { and end with the } and nothing else.
1. Each step must have a unique "id"
2. "depends_on" must reference existing step ids
3. If a step has no dependencies, use []
4. Use only the provided tools
5. Maximize parallel execution where possible
6. Do NOT create circular dependencies
7. Keep the plan minimal but complete

----------------------------------------
INPUT REFERENCES:

To use output from previous Steps, use:

{ "ref": "<step_id>.output" }

Example:
{
  "input": {
    "data": {"ref": "step_1.output"}
  }
}
IMPORTANT: 
when we failed during some steps adn when we retry to plan, keep the result of successful steps as input for next steps, and try to complete the goal as much as possible.
SO we don't need to retry to execute all the steps, we can just retry the failed steps.
Keep you are super smart, you can understand the context and the goal, and try to complete the goal with more efficiency.


----------------------------------------
AVAILABLE TOOLS:

{{TOOL_LIST}}

----------------------------------------
OUTPUT REQUIREMENTS:

- Output MUST be valid JSON
- Do NOT include markdown
- Do NOT include comments
- Do NOT include extra text

If you cannot produce a valid plan, output:

{ "error": "reason" }

If you are unsure, just keep asking questions.
If you need more information, ask for it.
If you need clarification, ask for it.
`
)

type Planning struct {
	Goal       string // 大写字段，可被JSON序列化
	Steps      []Step // 大写字段，可被JSON序列化
	Status     string // "pending", "running", "completed", "failed"，不需要序列化
	RetryCount int    // 重试次数，不需要序列化
}

type Step struct {
	Id        string                 `json:"id"`
	Tool      string                 `json:"tool"`
	Input     map[string]interface{} `json:"input"`
	DependsOn []string               `json:"depends_on"`
	Result    interface{}            `json:"result,omitempty"`
	Status    string                 `json:"status,omitempty"` // "pending", "running", "completed", "failed"
	Error     string                 `json:"error,omitempty"`
	InDegree  int                    `json:"indegree,omitempty"` // 依赖任务数
}

func NewPlanner(goal string) *Planning {
	return &Planning{
		Goal:       goal, // 使用 Goal 而不是 goal
		RetryCount: 6,    // 设置默认重试次数为 10 次
	}
}

func (p *Planning) AddStep(step Step) {
	p.Steps = append(p.Steps, step) // 使用 Steps 而不是 Steps
}

func (p *Planning) DoExe(ctx context.Context, rp string) (string, error) {
    var resultSteps string

    err := p.ParsePlan(rp)
    if err != nil {
        logging.Error("ParsePlan failed: %v", err)
        return "", err
    }

    // 执行计划
    err = p.Execute(ctx)
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


func (p *Planning)ParsePlan(jsonStr string) error {
	jsonStr = utils.ExtractJSON(jsonStr)
	var planData struct {
		Steps []Step `json:"Steps"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &planData); err != nil {
		return fmt.Errorf("failed to parse plan: %v", err)
	}

	p.Steps = planData.Steps

	return nil
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
	dialogOutChan := utils.OutputChan
	if dpOutchan, ok := ctx.Value(utils.DPDialogOutputChanString).(chan string); ok {
		dialogOutChan = dpOutchan
	}
	// 初始化所有步骤状态
	for i := range p.Steps {
		p.Steps[i].Status = "pending"
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
			dialogOutChan <- fmt.Sprintf("步骤 %s 执行失败: %v.跳过继续尝试执行剩余步骤...\n", p.Steps[current].Id, err)
			memory.AddUserMessage(chatID, fmt.Sprintf("步骤 %s 执行失败: %v.跳过继续尝试执行剩余步骤...\n", p.Steps[current].Id, err))
			// return fmt.Errorf("step %s failed: %v", p.Steps[current].Id, err)
		}

		logging.Info("步骤 %s 执行完成, 结果是: %v", p.Steps[current].Id, p.Steps[current].Result)
		dialogOutChan <- fmt.Sprintf("步骤 %s %s 执行完成, 结果是: %v\n", p.Steps[current].Id, p.Steps[current].Tool, p.Steps[current].Result)
		memory.AddUserMessage(chatID, fmt.Sprintf("步骤 %s 执行完成, 结果是: %v\n", p.Steps[current].Id, p.Steps[current].Result))
		// 使用反向依赖图更新依赖当前任务的其他任务的入度
		if dependentSteps, exists := reverseMap[p.Steps[current].Id]; exists {
			for _, depIndex := range dependentSteps {
				p.Steps[depIndex].InDegree--
				logging.Debug("更新步骤 %s 的依赖，剩余入度: %d", p.Steps[depIndex].Id, p.Steps[depIndex].InDegree)
				if p.Steps[depIndex].InDegree == 0 {
					queue = append(queue, depIndex)
					logging.Info("步骤 %s 所有依赖已完成，加入执行队列", p.Steps[depIndex].Id)
					dialogOutChan <- fmt.Sprintf("步骤 %s %s 所有依赖已完成，加入执行队列\n", p.Steps[depIndex].Id, p.Steps[depIndex].Tool)
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
			dialogOutChan <- utils.FinishString
			dialogOutChan <- fmt.Sprintf("执行阶段小结：步骤 %s %s 未完成，状态: %s", step.Id, step.Tool, step.Status)
			//return fmt.Errorf("some Steps are not completed, possibly due to circular dependencies")
		}
	}

	logging.Info("所有步骤尝试执行完成")
	//logging.Info("计划执行成功,plan: %v", p)
	dialogOutChan <- utils.FinishString
	return nil
}

func (p *Planning) ExecuteStep(ctx context.Context, stepIndex int) error {
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
