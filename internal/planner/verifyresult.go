package planner

import (
	"context"
	"errors"
	"fmt"
	"leiAgent/internal/memory"
	"leiAgent/internal/proxy"
	"leiAgent/utils"
)

func (p *Planning) VerifyResult(ctx context.Context, result string) (string, error) {

	chatID, ok := ctx.Value(utils.ChatIDString).(string)
	if !ok {
		return "", errors.New("chatID not found in context")
	}
	sql, err := memory.GetSqlInstance("")
	if err != nil {
		return "", err
	}
	subChatId, err := sql.GenerateSubChatIDWithChatId(chatID)
	if err != nil {
		return "", err
	}

	subctx, subcancel := context.WithCancel(context.Background())
	defer subcancel()

	dialogOutChan := utils.OutputChan
	if dpOutchan, ok := ctx.Value(utils.DPDialogOutputChanString).(chan string); ok {
		dialogOutChan = dpOutchan
	}
	reasoningOutChan := utils.ReasoningChan
	if dpReasoningOutchan, ok := ctx.Value(utils.DPReasoningOutputChanString).(chan string); ok {
		reasoningOutChan = dpReasoningOutchan
	}

	subctx = context.WithValue(subctx, utils.DPDialogOutputChanString, dialogOutChan)
	subctx = context.WithValue(subctx, utils.DPReasoningOutputChanString, reasoningOutChan)
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

	memory.AddSetSystemPrompt(subChatId, systemprompt)
	memory.AddUserMessage(subChatId, result)

	proxy := proxy.NewProxy(nil)
	response, err := proxy.Communicate(subctx)

	retrySteps := utils.ExtractJSON(response.Content)

	fmt.Println("分析结果: ", retrySteps)

	return retrySteps, err

}
