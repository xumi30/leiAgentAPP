package planner

import (
	"context"
	"encoding/json"
	"errors"
	"leiAgent/dataoperation"
	globalchannel "leiAgent/internal"
	"leiAgent/internal/memory"
	"leiAgent/internal/proxy"
	"leiAgent/logging"
	"leiAgent/utils"
)

const (
	PlannerPromotion = `You are a task planner for an execution engine.

Your job is to convert a user goal into a structured execution plan.

The plan MUST be a valid json object following the exact schema below.

You MUST NOT execute any tools.
You MUST NOT explain anything.
You MUST ONLY output json.

----------------------------------------
PLAN FORMAT:

{
  "goal": "string",
  "steps": [
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
Fist of all: MUST output json following the exact schema above, or it will be rejected。Start with the { and end with the } and nothing else.
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
when we failed during some steps and when we retry to plan, keep the result of successful steps as input for next steps, and try to complete the goal as much as possible.
SO we don't need to retry to execute all the steps, we can just retry the failed steps.
Keep you are super smart, you can understand the context and the goal, and try to complete the goal with more efficiency.


----------------------------------------
AVAILABLE TOOLS:

{{TOOL_LIST}}

----------------------------------------
OUTPUT REQUIREMENTS:

- Output MUST be valid json
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
	Goal       string `json:"goal"`
	Steps      []Step `json:"steps"`
	Status     string `json:"status"`      // "pending", "running", "completed", "failed"
	RetryCount int    `json:"retry_count"` // 重试次数

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

func GeneratePlan(ctx context.Context, goal string, toolInfo string) (*Planning, error) {

	chatId, ok := ctx.Value(utils.ChatIDString).(string)
	if !ok {
		logging.Error("无法从 context 中获取 chatId")
		return nil, errors.New("chatId not found in context")
	}
	dialogOutChan := globalchannel.GetGlobalDialogOutChannel(chatId)

	ctx = context.WithValue(ctx, utils.IsPlanningString, true)

	dialogOutChan <- utils.FinishString
	dialogOutChan <- "正在加载工具信息...\n"

	planInput := struct {
		Message string      `json:"message"`
		Goal    string      `json:"goal"`
		Tools   interface{} `json:"TOOL_LIST"`
	}{
		Message: "这是你的任务,你理解goal内容,进行你的任务规划。如果对任务意图有什么不明确,可以询问用户,直到你能明确任务目标,作为goal字段.然后开始你的规划。",
		Goal:    goal,
		Tools:   json.RawMessage(toolInfo),
	}
	planjson, err := json.MarshalIndent(planInput, "", "  ")
	if err != nil {
		logging.Error("序列化规划输入失败: %v", err)
		return nil, err
	}

	plannerProxy := proxy.NewProxy(nil)
	memory.GetLocalMemory().SetSystemPrompt(chatId, PlannerPromotion)
	logging.Info("planning系统提示词已加载...")

	memory.AddUserMessage(chatId, string(planjson))

	response, err := plannerProxy.Communicate(ctx)
	if err != nil {
		logging.Error("Response: %s", response.Content)
		dialogOutChan <- "规划失败: " + err.Error()

	}

	logging.Info("Agent 处理消息完成，返回结果: %s", response.Content)

	// unmarshal response.Content to Step[]
	var planner *Planning

	err = json.Unmarshal([]byte(response.Content), planner)
	if err != nil {
		logging.Error("解析规划结果失败: %v", err)
		return nil, err
	}
	planner.saveTodb()
	return planner, nil
}

// saveTodb 是 Planning 结构体的方法，用于将规划信息保存到数据库
// 该方法接收一个 Planning 类型的指针作为接收器
// 返回值类型为 error，表示保存操作可能出现的错误
func (p *Planning) saveTodb() error {
	// 记录日志信息，表明正在执行保存规划到数据库的操作
	logging.Info("正在保存规划到数据库...")
	// 调用 dataoperation 包中的 SavePlan 函数，传入规划的目标、状态和重试次数
	// 并将可能的错误返回给调用者
	return dataoperation.SavePlan(p.Goal, p.Status, p.RetryCount)
}
