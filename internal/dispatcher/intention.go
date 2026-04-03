package dispatcher

import (
	"context"
	"encoding/json"
	"errors"
	"leiAgent/internal/memory"
	"leiAgent/internal/proxy"
	"leiAgent/logging"
	"leiAgent/utils"
)

type Intention struct {
	Intent                string  `json:"intent"`
	Confidence            float64 `json:"confidence"`
	Reason                string  `json:"reason"`
	RequiresClarification bool    `json:"requires_clarification"`
	Goal                  string  `json:"goal", omitempty`
}

func ConfirmIntention(ctx context.Context, message string) (*Intention, error) {

	promotion := `You are an intent classification module in an AI agent system.

Your task is to classify the user's request into exactly ONE of the following categories:
MUST start with { and end with } and follow the JSON format strictly,or it will be considered as an error.

IF the request is ambiguous, set "requires_clarification" to true.

1. PLAN
- The request requires multi-step reasoning, task decomposition, or long-term execution.
- The task cannot be completed in a single tool call.
- The user intent implies a workflow, pipeline, or goal-oriented process.
- Examples: "Build a trading bot", "Help me plan a trip", "Analyze and summarize a dataset step by step"

2. TOOL  
- The request can be completed with a single tool/function call.
- No complex reasoning or planning is required.
- The request is atomic and well-defined.
- Examples: "Search for latest Bitcoin price", "Translate this sentence to Chinese", "Get weather in New York"

3. CHAT
- The request is conversational, explanatory, or opinion-based.
- No tool usage is required.
- Examples: "What is blockchain?", "Do you think AI is dangerous?", "Explain Kubernetes simply"

4. SWITCH
- User explicitly requests to change the current mode/intention.
- Keywords: "切换到", "switch to", "change to", "mode"
- Examples: "切换到工具模式", "switch to tool mode", "change to planning mode"

---

## Output Requirements (STRICT)

You MUST return a JSON object with the following structure:

{
  "intent": "PLAN | TOOL | CHAT | SWITCH",
  "confidence": 0.0-1.0,
  "reason": "short explanation",
  "requires_clarification": true | false
}

---

## Additional Rules

- If uncertain about intent classification, set "requires_clarification" to true.
- Prefer TOOL over PLAN if a single tool can reasonably satisfy the request.
- Prefer CHAT if no execution is required.
- For SWITCH intent, only classify when user explicitly mentions mode switching.
- DO NOT output anything other than JSON.`
	js := toolsInfo()
	intentInput := struct {
		Message string      `json:"message"`
		Tools   interface{} `json:"TOOL_LIST"`
	}{
		Message: message,
		Tools:   json.RawMessage(js),
	}
	intentJSON, err := json.MarshalIndent(intentInput, "", "  ")
	if err != nil {
		logging.Error("序列化规划输入失败: %v", err)
	}

	message = `这个json的message这是用户请求,TOOL_LIST是可用的工具列表,根据这个信息，帮我判断用户请求的意图:` + string(intentJSON)

	p := proxy.NewProxy(nil)
	memory.GetLocalMemory().SetSystemPrompt(ctx.Value(utils.ChatIDString).(string), promotion)
	memory.AddUserMessage(ctx.Value(utils.ChatIDString).(string), message)

	response, err := p.Communicate(ctx)
	if err != nil {
		logging.Error("Response error: %v", err)
		return nil, err
	}

	result, err := parseIntention(utils.ExtractJSON(response.Content))
	if err != nil {
		logging.Error("Failed to parse intention: %v", err)
		return nil, err
	}
	result.Goal = message // 保存原始请求作为目标
	return result, nil

}

func parseIntention(data string) (*Intention, error) {
	var i Intention

	err := json.Unmarshal([]byte(data), &i)
	if err != nil {
		return nil, err
	}

	// ✅ 基础校验
	if i.Intent == "" {
		return nil, errors.New("intent is empty")
	}

	if i.Intent != utils.ChatModeString && i.Intent != utils.ToolModeString && i.Intent != utils.PlanModeString && i.Intent != utils.SwitchModeString {
		return nil, errors.New("invalid intent type")
	}

	return &i, nil
}
