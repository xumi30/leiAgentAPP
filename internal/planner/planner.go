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

const PlannerPromotion = `
You are a task planner for a DAG execution engine.

Your job is to convert a user goal into a structured execution plan.

The output MUST be a JSON object that can be directly deserialized into predefined Go structs.

You MUST NOT execute tools.
You MUST NOT explain anything.
You MUST ONLY output JSON.

----------------------------------------
STRICT JSON SCHEMA (MUST FOLLOW EXACTLY):

{
  "goal": "string",
  "status": "pending",
  "retry_count": 0,
  "steps": [
    {
      "id": "step_1",
      "tool": "string",
      "depends_on": ["step_x"],
      "input": {
        "param_name": {
          "ref_step_id": "string",
          "ref_step_out_field": "string",
          "step_input_value": any
        }
      },
      "status": "pending",
      "error": "",
      "indegree": 0
    }
  ]
}

----------------------------------------
JSON STRICTNESS (HARD REQUIREMENTS):

- Output ONLY a JSON object
- MUST start with "{" and end with "}"
- Use double quotes for ALL keys and string values
- NO trailing commas
- NO markdown
- NO explanations
- Strings MUST NOT contain raw line breaks, use \\n

----------------------------------------
PLAN RULES:

1. Each step MUST have a unique "id" (step_1, step_2, ...)
2. "depends_on" MUST reference valid step IDs
3. If no dependency, use []
4. NO circular dependencies
5. Maximize parallel execution where possible
6. Keep plan minimal but complete

----------------------------------------
INPUT STRUCTURE (CRITICAL):

Each input parameter MUST follow:

{
  "param": {
    "ref_step_id": "",
    "ref_step_out_field": "",
    "step_input_value": value
  }
}

----------------------------------------
INPUT RULES (STRICT):

### CASE 1: Direct value
- Use when value is static

{
  "query": {
    "ref_step_id": "",
    "ref_step_out_field": "",
    "step_input_value": "weather in Hangzhou"
  }
}

### CASE 2: Reference previous step output

{
  "latitude": {
    "ref_step_id": "step_1",
    "ref_step_out_field": "latitude",
    "step_input_value": null
  }
}

STRICT RULES:

- MUST use ref_step_id if data comes from another step
- MUST set step_input_value = null in this case
- MUST include that step in depends_on

- NEVER mix:
  ref_step_id != "" AND step_input_value != null ❌

----------------------------------------
DEPENDENCY COMPLETENESS (CRITICAL):

- If a step logically requires upstream data,
  it MUST:

  1. include it in depends_on
  2. reference it in input

- Example:
  Calculating time MUST depend on current time

- NEVER omit required inputs

----------------------------------------
OUTPUT FIELD RESTRICTIONS (CRITICAL):

- ref_step_out_field MUST be a SIMPLE top-level field

- DO NOT use:
  - nested paths (a.b.c)
  - array indexing ([0], [1])

- If complex data is needed:
  pass the entire object instead

----------------------------------------
NO TEMPLATE STRINGS (CRITICAL):

- DO NOT generate strings like:
  {{step_1.xxx}} or any placeholders

- DO NOT embed step outputs inside strings

- ALL cross-step data MUST use structured inputValue

----------------------------------------
STRUCTURED DATA ONLY:

- If combining multiple data sources:
  DO NOT build formatted strings

- Instead, pass structured JSON:

Example:

"content": {
  "ref_step_id": "",
  "ref_step_out_field": "",
  "step_input_value": {
    "date": {
      "ref_step_id": "step_2",
      "ref_step_out_field": "calculated_time"
    },
    "weather": {
      "ref_step_id": "step_4",
      "ref_step_out_field": "forecast"
    }
  }
}

----------------------------------------
TOOL RESPONSIBILITY (VERY IMPORTANT):

- Planner MUST NOT:
  - format text
  - generate markdown
  - produce final user content

- Planner MUST:
  - define data flow
  - connect tool outputs

- Tools are responsible for rendering and formatting

----------------------------------------
DAG CONSISTENCY:

- indegree MUST equal len(depends_on)
- Every ref_step_id MUST exist
- Every dependency MUST be resolvable

----------------------------------------
RETRY-AWARE PLANNING:

If previous execution context exists:

- DO NOT recreate successful steps
- Reuse their outputs via ref_step_id
- ONLY replan failed steps

----------------------------------------
AVAILABLE TOOLS:

{{TOOL_LIST}}

----------------------------------------
FINAL SANITY CHECK (MANDATORY):

Before output, verify:

1. JSON is valid
2. No template placeholders exist
3. All dependencies are correct
4. No missing required inputs
5. indegree is correct

----------------------------------------
IF CLARIFICATION IS NEEDED:

Output ONLY:

{ "error": "your question" }

DO NOT include goal or steps in that case.
`

type Planning struct {
	Goal       string `json:"goal"`
	Steps      []Step `json:"steps"`
	Status     string `json:"status"`      // "pending", "running", "completed", "failed"
	RetryCount int    `json:"retry_count"` // 重试次数

}

type Step struct {
	Id        string                `json:"id"`
	Tool      string                `json:"tool"`
	Input     map[string]inputValue `json:"input"`
	DependsOn []string              `json:"depends_on"`
	Result    interface{}           `json:"result,omitempty"`
	Status    string                `json:"status,omitempty"` // "pending", "running", "completed", "failed"
	Error     string                `json:"error,omitempty"`
	InDegree  int                   `json:"indegree,omitempty"` // 依赖任务数
}

type inputValue struct {
	RefStepID       string      `json:"ref_step_id"`        // 引用步骤的ID,例如 "step_1",如果不要引用,则为空字符串
	RefStepOutField string      `json:"ref_step_out_field"` // 引用步骤的输出字段,例如 "latitude",如果不要引用,则为空字符串
	StepInputValue  interface{} `json:"step_input_value"`   // 引用步骤的输出值,在执行步骤时填充.如果不需要引用,则为空值。也可作为输入值直接传入下一步骤的输入。
}

func GeneratePlan(ctx context.Context, goal string, toolInfo string) (*Planning, error) {

	chatId, ok := ctx.Value(utils.ChatIDString).(string)
	if !ok {
		logging.Error("无法从 context 中获取 chatId")
		return nil, errors.New("chatId not found in context")
	}

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
			globalchannel.SendAssitantMessageOnce(ctx, fmt.Sprintf("规划请求失败: %v", err.Error()))
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
			globalchannel.SendAssitantMessageOnce(ctx, fmt.Sprintf("规划结果不是合法 JSON（已自动重试 %d 次）：%v", attempt, err))
			if attempt < maxPlanParseAttempts {
				// 把错误原因带回给模型，让它修正 JSON 后“只输出 JSON”重试一次。
				preview := rawJSON

				memory.AddUserMessage(chatId, fmt.Sprintf(
					"你上一次输出不是合法 JSON，解析错误：%v。\n请严格按系统提示的 schema 重新输出，注意：所有 key 必须有双引号、不要尾逗号、depends_on 必须是数组（空用 []）、不要输出 ```json 等任何额外文本。\n上一版：\n%s",
					err, preview,
				))
				continue
			}
			break
		}

		if len(planner.Steps) == 0 {
			globalchannel.SendAssitantMessageOnce(ctx, "你没有生成任何执行步骤，请重新规划")
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
