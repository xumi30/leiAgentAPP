package planner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"leiAgent/dataoperation"
	"leiAgent/internal/globalchannel"
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
First of all: you MUST output a single JSON object following the exact schema above, or it will be rejected. Start with "{" and end with "}" and nothing else.
JSON STRICTNESS (hard requirements):
- Use double quotes for ALL keys and ALL string values
- No trailing commas
- "depends_on" MUST be an array (use [] when empty)
- Do not output markdown code fences (triple backticks), e.g. code blocks
- Do not output any text before/after the JSON
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
- Inside any string value, never use real line breaks; use \n (backslash + letter n) in the JSON text instead
- Do NOT include markdown
- Do NOT include comments
- Do NOT include extra text

If you cannot produce a valid plan yet (need clarification), output ONLY:

{ "error": "your question to the user in their language" }

In that case do not include "goal" or "steps". For a runnable plan, never use the "error" key.
`

	// PlanSummarySystemPrompt 在计划执行结束后，将结果 JSON 转为用户可读总结
	PlanSummarySystemPrompt = `You summarize a completed multi-step plan execution for the end user.

Input context contains JSON with "goal", "steps" (id, tool, status, result, error fields), and plan status.

Rules:
- Write in the same language as the user's goal when you can infer it.
- Use Markdown: headings and bullet lists are encouraged.
- Cover: what they asked for; what each step did and whether it succeeded; key outcomes (summarize large results, do not paste huge blobs).
- End with overall outcome (full success / partial / failed) and short next-step suggestions if helpful.
- Do not repeat the full raw JSON.`
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

	plannerProxy, err := proxy.NewProxy(nil)
	if err != nil {
		return nil, err
	}
	memory.GetLocalMemory().SetSystemPrompt(chatId, PlannerPromotion)
	logging.Info("planning系统提示词已加载...")

	memory.AddUserMessage(chatId, string(planjson))

	const maxPlanParseAttempts = 3
	var lastParseErr error
	for attempt := 1; attempt <= maxPlanParseAttempts; attempt++ {
		response, err := plannerProxy.Communicate(ctx)
		if err != nil {
			logging.Error("规划请求失败: %v", err)
			dialogOutChan <- &globalchannel.Message{Content: "规划失败: " + err.Error(), Role: utils.MessageRoleAssistant, IsFinished: false}
			return nil, err
		}

		logging.Info("Agent 处理消息完成（plan attempt=%d），返回结果: %s", attempt, response.Content)

		rawJSON := utils.PrepareLLMJSON(response.Content)

		var probe struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal([]byte(rawJSON), &probe); err == nil && strings.TrimSpace(probe.Error) != "" {
			return nil, fmt.Errorf("需要先确认：%s", probe.Error)
		}

		var planner Planning
		if err := json.Unmarshal([]byte(rawJSON), &planner); err != nil {
			lastParseErr = err
			logging.Error("解析规划结果失败（plan attempt=%d）: %v", attempt, err)

			if attempt < maxPlanParseAttempts {
				// 把错误原因带回给模型，让它修正 JSON 后“只输出 JSON”重试一次。
				preview := rawJSON
				if len(preview) > 2000 {
					preview = preview[:2000] + "...(truncated)"
				}
				memory.AddUserMessage(chatId, fmt.Sprintf(
					"你上一次输出不是合法 JSON，解析错误：%v。\n请严格按系统提示的 schema 重新输出，注意：所有 key 必须有双引号、不要尾逗号、depends_on 必须是数组（空用 []）、不要输出 ```json 等任何额外文本。\n上一版输出（节选）：\n%s",
					err, preview,
				))
				continue
			}
			break
		}

		if len(planner.Steps) == 0 {
			return nil, fmt.Errorf("模型未生成任何执行步骤；若你只是在讨论或补充信息，可用较短说法触发模式重判，或把任务写得更具体后再发")
		}
		planner.Status = "pending"
		if err := planner.saveTodb(chatId); err != nil {
			logging.Error("保存规划到数据库失败: %v", err)
			return nil, err
		}
		return &planner, nil
	}

	if lastParseErr != nil {
		return nil, fmt.Errorf("规划结果不是合法 JSON（已自动重试 %d 次）：%v", maxPlanParseAttempts, lastParseErr)
	}
	return nil, fmt.Errorf("生成计划失败：未知错误")
}

func (p *Planning) saveTodb(chatId string) error {

	logging.Info("正在保存规划到数据库...")

	return dataoperation.SavePlan(chatId, p.Goal, p.Status, p.RetryCount)
}
