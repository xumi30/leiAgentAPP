package planner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"leiAgent/dataoperation"
	"leiAgent/internal/provider/openaistyle"
	"leiAgent/internal/proxy"
	"leiAgent/logging"
	"leiAgent/utils"
)

// maxVerifyAttempts 模型返回非法/不完整 JSON 时的最大请求次数（含首次），避免无限重试。
const maxVerifyAttempts = 6

const VerifySystemPrompt = `
You are a recovery planner for a DAG execution engine.

Your job is to analyze a previously executed plan (with runtime results),
identify failed steps, and produce a corrected plan for the next execution.

You MUST ONLY output JSON.
You MUST NOT execute tools.
You MUST NOT explain anything.

----------------------------------------
INPUT (EXECUTION STATE):

You receive a JSON object with:

- "goal"
- "status"
- "retry_count"
- "steps": array

Each step has:

{
  "id": "string",
  "tool": "string",
  "depends_on": [],
  "input": {
    "param": {
      "ref_step_id": "string",
      "ref_step_out_field": "string",
      "step_input_value": any
    }
  },
  "status": "pending" | "running" | "completed" | "failed",
  "result": any,
  "error": "string"
}

----------------------------------------
FAILURE DETECTION:

A step is FAILED if:

- status == "failed"
- OR error is not empty

----------------------------------------
CORE PRINCIPLES (CRITICAL):

1. DO NOT recreate the entire plan
2. KEEP all successful steps unchanged
3. ONLY fix failed steps and necessary downstream steps
4. MINIMIZE changes

----------------------------------------
INPUT STRUCTURE (STRICT):

Each input MUST follow:

{
  "param": {
    "ref_step_id": "string",
    "ref_step_out_field": "string",
    "step_input_value": any
  }
}

----------------------------------------
INPUT VALUE RULE (CRITICAL):

Each inputValue MUST follow ONE of two modes:

MODE 1: DIRECT VALUE

{
  "ref_step_id": "",
  "ref_step_out_field": "",
  "step_input_value": value
}

MODE 2: REFERENCE VALUE

{
  "ref_step_id": "step_1",
  "ref_step_out_field": "output_field",
  "step_input_value": null
}

STRICT RULES:

- EXACTLY ONE mode must be used
- NEVER mix:
  ref_step_id != "" AND step_input_value != null ❌

- If ref_step_id != "":
  step_input_value MUST be null

- If direct value:
  ref_step_id MUST be ""

----------------------------------------
DEPENDENCY RULES:

- If ref_step_id is used:
  MUST include it in depends_on

- MUST NOT create missing or invalid dependencies

----------------------------------------
NO NESTED inputValue (CRITICAL):

- step_input_value MUST NOT contain nested inputValue objects

- INVALID:

{
  "step_input_value": {
    "a": {
      "ref_step_id": "step_1"
    }
  }
}

- Instead use separate parameters

----------------------------------------
OUTPUT FIELD RULES:

- ref_step_out_field MUST be simple field name
- DO NOT use:
  a.b.c
  array[0]

----------------------------------------
NO TEMPLATE STRINGS:

- DO NOT generate:
  {{step_x.xxx}}

----------------------------------------
DECISIONS:

For each failed step, output:

{
  "step_id": "string",
  "reason": "string",
  "decision": "retry" | "skip" | "abort"
}

RULES:

- retry → fix and rerun this step
- skip → remove step if goal still achievable
- abort → stop entire plan

- DO NOT retry completed steps
- DO NOT retry a step if its dependencies are still failed

----------------------------------------
GOALSTEPS (IMPORTANT):

You MUST output the FULL next execution plan.

- INCLUDE:
  - unchanged successful steps
  - corrected failed steps

- REMOVE skipped steps

- If abort:
  Goalsteps MUST be []

----------------------------------------
OUTPUT FORMAT (STRICT):

{
  "analysis": [
    {
      "step_id": "string",
      "reason": "string",
      "decision": "retry"
    }
  ],
  "Goalsteps": [
    {
      "id": "string",
      "tool": "string",
      "depends_on": [],
      "input": {
        "param": {
          "ref_step_id": "",
          "ref_step_out_field": "",
          "step_input_value": null
        }
      }
    }
  ]
}

----------------------------------------
FINAL CHECK (MANDATORY):

1. JSON is valid
2. All inputs follow structure
3. No nested inputValue
4. No template strings
5. All dependencies are valid
6. No mixed ref/value usage

----------------------------------------
IF NOTHING TO FIX:

Return:

{
  "analysis": [],
  "Goalsteps": []
}
`

func (p *Planning) VerifyResult(ctx context.Context, result string) (string, error) {
	logging.Info("开始验证执行结果")

	chatID, ok := ctx.Value(utils.ChatIDString).(string)
	if !ok {
		return "", errors.New("chatID not found in context")
	}
	sql := dataoperation.GetSqlInstance()
	if sql == nil {
		return "", errors.New("database not available")
	}

	workingMessages := []openaistyle.ChatMessage{
		{
			Role:    openaistyle.RoleSystem,
			Content: VerifySystemPrompt,
		},
		{
			Role:    openaistyle.RoleUser,
			Content: "this is the execution state of this plan:\n\n" + result,
		},
		{
			Role:    openaistyle.RoleUser,
			Content: "this is the prompt of the planner who produced this plan and he made miskakes(help him to fix them):\n" + PlannerPromotion,
		},
	}

	finalResult, err := p.verifyResultWithRetry(ctx, chatID, workingMessages)
	if err != nil {
		return "", err
	}

	return finalResult, nil
}

func (p *Planning) verifyResultWithRetry(ctx context.Context, chatID string, workingMessages []openaistyle.ChatMessage) (string, error) {
	var lastErr error

	for attempt := 1; attempt <= maxVerifyAttempts; attempt++ {
		raw, err := sendVerifyMessage(ctx, chatID, workingMessages)
		if err != nil {
			return "", err
		}

		var goalsteps map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &goalsteps); err != nil {
			logging.Error("解析JSON失败: %v", err)
			lastErr = fmt.Errorf("解析JSON失败: %w", err)
			workingMessages = append(workingMessages, openaistyle.ChatMessage{
				Role:    openaistyle.RoleUser,
				Content: "执行结果分析失败：上次返回不是合法 JSON，请只输出一节严格符合约定的 JSON（含 analysis 与 Goalsteps）。",
			})
			continue
		}

		if _, ok := goalsteps["analysis"]; !ok {
			logging.Error("返回的JSON缺少 'analysis' 字段")
			lastErr = fmt.Errorf("返回的JSON缺少 'analysis' 字段")
			workingMessages = append(workingMessages, openaistyle.ChatMessage{
				Role:    openaistyle.RoleUser,
				Content: "执行结果分析失败,返回的JSON缺少 'analysis' 字段",
			})
			continue
		}
		if _, ok := goalsteps["Goalsteps"]; !ok {
			logging.Error("返回的JSON缺少 'Goalsteps' 字段")
			lastErr = fmt.Errorf("返回的JSON缺少 'Goalsteps' 字段")
			workingMessages = append(workingMessages, openaistyle.ChatMessage{
				Role:    openaistyle.RoleUser,
				Content: "执行结果分析失败,返回的JSON缺少 'Goalsteps' 字段",
			})
			continue
		}

		var goalStepsJSON []byte
		if goalstepsStr, ok := goalsteps["Goalsteps"].(string); ok {
			goalStepsJSON = []byte(utils.EscapeRawNewlinesInJSONStrings(goalstepsStr))
		} else {
			goalStepsJSON, err = json.Marshal(goalsteps["Goalsteps"])
			if err != nil {
				logging.Error("序列化Goalsteps失败: %v", err)
				lastErr = fmt.Errorf("序列化Goalsteps失败: %w", err)
				workingMessages = append(workingMessages, openaistyle.ChatMessage{
					Role:    openaistyle.RoleUser,
					Content: "执行结果分析失败：Goalsteps 字段无法序列化为 JSON，请修正后重试。",
				})
				continue
			}
		}

		return string(goalStepsJSON), nil
	}

	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("验证结果在 %d 次尝试后仍失败", maxVerifyAttempts)
}

func sendVerifyMessage(ctx context.Context, chatID string, workingMessages []openaistyle.ChatMessage) (string, error) {
	px, err := proxy.NewProxy(nil)
	if err != nil {
		return "", err
	}
	reqCtx := context.WithValue(ctx, utils.ChatIDString, chatID)
	reqCtx = context.WithValue(reqCtx, utils.IsStreamString, false)
	reqCtx = context.WithValue(reqCtx, utils.SkipDialogToUIString, true)
	response, err := px.CommunicateWithMessages(reqCtx, workingMessages)

	if err != nil {
		return "", err
	}

	retrySteps := utils.PrepareLLMJSON(response.Content)
	logging.Info("VerifyResult: %s", retrySteps)
	return retrySteps, nil
}
