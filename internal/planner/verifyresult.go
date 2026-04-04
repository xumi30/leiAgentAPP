package planner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"leiAgent/internal/memory"
	"leiAgent/internal/memory/sqlmemory"
	"leiAgent/internal/proxy"
	"leiAgent/logging"
	"leiAgent/utils"
)

func (p *Planning) VerifyResult(ctx context.Context, result string) (string, error) {

	chatID, ok := ctx.Value(utils.ChatIDString).(string)
	if !ok {
		return "", errors.New("chatID not found in context")
	}
	sql, err := sqlmemory.GetSqlInstance("")
	if err != nil {
		return "", err
	}
	subChatId, err := sql.GenerateSubChatIDWithChatId(chatID)
	if err != nil {
		return "", err
	}

	subctx, subcancel := context.WithCancel(context.Background())
	defer subcancel()

	// 将subChatId放入subctx中
	subctx = context.WithValue(subctx, utils.ChatIDString, subChatId)

	select {
	case <-ctx.Done():
		subcancel()
		return "", ctx.Err()
	default:
	}

	systemprompt := `
	You are an execution recovery agent in a task planning system.

Your job is to analyze execution results, detect failures, and decide whether and how to retry failed steps.

## Input JSON
You will receive a task execution state in the following format:

{
  "Goal": string,
  "Steps": Step[]
}

Each Step contains:
- id: string
- tool: string
- input: object
- depends_on: string[]
- status: "completed" | "failed" | "pending"
- result: {
    success: boolean,(this may be missing if the tool does not return it, in that case rely on status)
    ... (tool-specific fields)
  }
- error: error message if failed

---

## Your Responsibilities

### 1. Identify Failed Steps
- Find all steps where:
  - status == "failed"
  OR
  - result.success == false

---

### 2. Root Cause Analysis
For each failed step:
- Determine the most likely failure reason:
  - Invalid input
  - Tool misuse
  - Missing dependency
  - Partial execution (e.g., truncated content)
  - Environment/runtime issue

Be concise but precise.

---

### 3. Retry Decision

For each failed step, decide:

- "retry": if the issue is fixable
- "skip": if retry is useless
- "abort": if the whole plan should stop

Rules:
- Do NOT retry successful steps
- Do NOT retry if dependencies are failed
- Avoid infinite retries (assume max 2 retries)

---

### 4. Fix + Regenerate Step

If retry == true:
- Generate a corrected version of the step
- Keep the SAME step id
- Fix ONLY the problematic fields (do not rewrite everything blindly)
- Ensure:
  - input is valid
  - dependencies are respected
  - content is complete (no truncation)
- The returned Goalsteps should be the corrected complete list of steps, including all steps (both successful and failed), but the failed steps have been corrected for retry.
---

### 5. Output Format (STRICT JSON)

Return:

{
  "analysis": [
    {
      "step_id": "...",
      "reason": "...",
      "decision": "retry | skip | abort"
    }
  ],
  "Goalsteps": [
    {
      "id": "...",
      "tool": "...",
      "input": { ... },
      "depends_on": [...]
    }
  ]
}

---

## Constraints

- Do NOT modify successful steps
- Do NOT regenerate the whole plan
- Be deterministic and minimal
- Prefer partial fixes over full rewrites
- If no failures → return empty retry_steps

---

## Now analyze the following input:

{{EXECUTION_STATE_JSON}}
	
	`

	retryTimes := 0
	// 清楚之前的对话历史，避免干扰
	memory.GetLocalMemory().Clear(subChatId)
	memory.SetSystemPrompt(subChatId, systemprompt)
	memory.AddUserMessage(subChatId, result)

	proxy := proxy.NewProxy(nil)
	response, err := proxy.Communicate(subctx)

	retrySteps := utils.ExtractJSON(response.Content)
	goalsteps := map[string]interface{}{}

	err = json.Unmarshal([]byte(retrySteps), &goalsteps)
	if err != nil {
		logging.Error("解析JSON失败: %v", err)
		return "", fmt.Errorf("解析JSON失败: %v", err)
	}
	// 判断 goalsteps 中是否包含 "analysis" 和 "Goalsteps" 字段
	if _, ok := goalsteps["analysis"]; !ok {
		logging.Error("返回的JSON缺少 'analysis' 字段")
		
		//memory.AddUserMessage(subChatId, "执行结果分析失败，返回的JSON缺少 'analysis' 字段")
		if retryTimes < 3 {
			retryTimes++
			p.VerifyResult(ctx, result)
		}
		return result, fmt.Errorf("返回的JSON缺少 'analysis' 字段")
	}
	if _, ok := goalsteps["Goalsteps"]; !ok {
		logging.Error("返回的JSON缺少 'Goalsteps' 字段")
		//memory.AddUserMessage(subChatId, "执行结果分析失败，返回的JSON缺少 'Goalsteps' 字段")
		if retryTimes < 3 {
			retryTimes++
			p.VerifyResult(ctx, result)
		}
		return result, fmt.Errorf("返回的JSON缺少 'analysis' 字段")
	}
	fmt.Println("分析结果: ", goalsteps["analysis"])

	// 处理 Goalsteps 字段
	var goalStepsJSON []byte

	// 判断 Goalsteps 的类型
	if goalstepsStr, ok := goalsteps["Goalsteps"].(string); ok {
		// 如果已经是字符串，直接使用
		goalStepsJSON = []byte(goalstepsStr)
	} else {
		// 如果是其他类型（如数组），则序列化为 JSON
		goalStepsJSON, err = json.Marshal(goalsteps["Goalsteps"])
		if err != nil {
			logging.Error("序列化Goalsteps失败: %v", err)
			return "", fmt.Errorf("序列化Goalsteps失败: %v", err)
		}
	}

	return string(goalStepsJSON), nil

}
