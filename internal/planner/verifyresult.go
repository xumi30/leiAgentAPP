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

// maxVerifyAttempts 模型返回非法/不完整 JSON 时的最大请求次数（含首次），避免无限重试。
const maxVerifyAttempts = 6

const systemprompt = `You are an execution-recovery agent for the same task planner as the planning phase.

Your job is to read a post-run plan JSON (what the engine already executed), find failures, and emit corrected steps for the next execution attempt.

You MUST NOT execute any tools.
You MUST NOT explain anything outside the JSON.
You MUST ONLY output one json object.

The user message is the execution state: same shape as a plan plus per-step runtime fields. Analyze only that payload; do not ask for more input.

----------------------------------------
EXECUTION STATE (input):

Top-level fields include at least:
  "goal", "steps", "status", "retry_count"

Each element of "steps" matches the plan step schema and may include:
  "id", "tool", "input", "depends_on",
  "status" ("pending"|"running"|"completed"|"failed"),
  "result" (object; "success" may be missing—then trust "status"),
  "error" when failed

----------------------------------------
FAILURE DETECTION:

Treat a step as failed if:
  - "status" == "failed", OR
  - "result"."success" is explicitly false

----------------------------------------
RETRY POLICY (align with planning):

When some steps failed and the system retries, keep successful steps' outcomes in mind: downstream "input" may use refs to prior step output. Prefer fixing only failed steps and anything that must change for dependencies—do not rerun the whole plan from scratch unless necessary.

To reference output from a previous step inside "input", use the same convention as planning:

  { "ref": "<step_id>.output" }

Do not invent new ref shapes.

----------------------------------------
DECISIONS (per failed step):

For each failed step, output one analysis row with "decision" exactly one of:
  "retry" | "skip" | "abort"

Rules:
1. Do not mark "retry" for steps that already "completed".
2. Do not mark "retry" for a step if any of its "depends_on" steps failed (fix or skip/abandon dependencies first).
3. Prefer minimal edits: same "id", change only "input" / "depends_on" / "tool" when needed to fix the root cause.
4. "depends_on" must only list existing step ids in "Goalsteps"; no circular dependencies.
5. If the whole run must stop, use "abort" and set "Goalsteps" to [] with a clear reason in "analysis".
6. If there is nothing to fix, return "analysis": [] and "Goalsteps": [].

----------------------------------------
VERIFY OUTPUT FORMAT:

MUST be valid json. Start with { and end with } and nothing else.
Do NOT include markdown. Do NOT include comments. Do NOT include extra text.

Keys MUST match exactly (parser is case-sensitive):

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
      "input": {}
    }
  ]
}

"Goalsteps" is the full ordered list of steps to run next: unchanged successful steps plus corrected failed steps (when decision is "retry"). Omit or drop skipped steps according to "skip" decisions if the goal can still be met; on "abort", use [].
`

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

	subctx, subcancel := context.WithCancel(ctx)
	defer subcancel()

	subctx = context.WithValue(subctx, utils.ChatIDString, subChatId)
	memory.GetLocalMemory().Clear(subChatId)
	memory.SetSystemPrompt(subChatId, systemprompt)
	memory.AddUserMessage(subChatId, result)

	finalResult, err := p.verifyResultWithRetry(subctx, subChatId, result)
	if err != nil {
		return "", err
	}
	memory.GetLocalMemory().Clear(subChatId)
	return finalResult, nil
}

func (p *Planning) verifyResultWithRetry(ctx context.Context, subChatId, result string) (string, error) {
	var lastErr error

	for attempt := 1; attempt <= maxVerifyAttempts; attempt++ {
		raw, err := sendMessage(ctx)
		if err != nil {
			return "", err
		}

		var goalsteps map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &goalsteps); err != nil {
			logging.Error("解析JSON失败: %v", err)
			lastErr = fmt.Errorf("解析JSON失败: %w", err)
			memory.AddUserMessage(subChatId, "执行结果分析失败：上次返回不是合法 JSON，请只输出一节严格符合约定的 JSON（含 analysis 与 Goalsteps）。")
			continue
		}

		if _, ok := goalsteps["analysis"]; !ok {
			logging.Error("返回的JSON缺少 'analysis' 字段")
			lastErr = fmt.Errorf("返回的JSON缺少 'analysis' 字段")
			memory.AddUserMessage(subChatId, "执行结果分析失败,返回的JSON缺少 'analysis' 字段")
			continue
		}
		if _, ok := goalsteps["Goalsteps"]; !ok {
			logging.Error("返回的JSON缺少 'Goalsteps' 字段")
			lastErr = fmt.Errorf("返回的JSON缺少 'Goalsteps' 字段")
			memory.AddUserMessage(subChatId, "执行结果分析失败,返回的JSON缺少 'Goalsteps' 字段")
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
				memory.AddUserMessage(subChatId, "执行结果分析失败：Goalsteps 字段无法序列化为 JSON，请修正后重试。")
				continue
			}
		}

		return string(goalStepsJSON), nil
	}

	if lastErr != nil {
		return result, lastErr
	}
	return result, fmt.Errorf("验证结果在 %d 次尝试后仍失败", maxVerifyAttempts)
}

func sendMessage(ctx context.Context) (string, error) {

	px, err := proxy.NewProxy(nil)
	if err != nil {
		return "", err
	}
	response, err := px.Communicate(ctx)

	if err != nil {
		return "", err
	}

	retrySteps := utils.PrepareLLMJSON(response.Content)
	logging.Info("VerifyResult: %s", retrySteps)
	return retrySteps, nil
}
