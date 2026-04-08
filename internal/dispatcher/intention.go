package dispatcher

import (
	"context"
	"encoding/json"
	"errors"
	"leiAgent/internal/memory"
	"leiAgent/internal/proxy"
	"leiAgent/logging"
	"leiAgent/utils"
	"strings"
	"unicode/utf8"
)

type Intention struct {
	Intent                string  `json:"intent"`
	Confidence            float64 `json:"confidence"`
	Reason                string  `json:"reason"`
	RequiresClarification bool    `json:"requires_clarification"`
	Goal                  string  `json:"goal,omitempty"`
	Content               string  `json:"content,omitempty"`
}

func ConfirmIntention(ctx context.Context, message string) (*Intention, error) {
	userMessage := message

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
- If the intent is TOOL, you need to return a JSON object with "intent": "TOOL" and "tooltopic".No need to call any tool.

3. CHAT
- The request is conversational, explanatory, or opinion-based.
- No tool usage is required.
- Examples: "What is blockchain?", "Do you think AI is dangerous?", "Explain Kubernetes simply"
- If the intent is chat, just reply to the user directly with the content in the json. No need to call any tool.


---

## Output Requirements (STRICT)

You MUST return a JSON object with the following structure:

{
  "intent": "PLAN | TOOL | CHAT ",
  "confidence": 0.0-1.0,
  "reason": "short explanation",
  "requires_clarification": true | false
  "content": "optional, only used when intent is CHAT"
  "tooltopic": "optional, only used when intent is TOOL"
}

---

## Additional Rules

- If uncertain about intent classification, set "requires_clarification" to true.
- Prefer TOOL over PLAN if a single tool can reasonably satisfy the request.
- Prefer CHAT if no execution is required.
- For SWITCH intent, only classify when user explicitly mentions mode switching.
- DO NOT output anything other than JSON.`
	js := getToolsSimpleInfo()

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

	message = `这个json的message这是用户请求,TOOL_LIST是可用的工具列表的简单信息,根据这个信息，帮我判断用户请求的意图:` + string(intentJSON)

	chatIDVal := ctx.Value(utils.ChatIDString)
	chatIDStr, ok := chatIDVal.(string)
	if !ok || strings.TrimSpace(chatIDStr) == "" {
		return nil, errors.New("ConfirmIntention: context 缺少有效的 chatID")
	}

	p, err := proxy.NewProxy(nil)
	if err != nil {
		return nil, err
	}
	memory.GetLocalMemory().SetSystemPrompt(chatIDStr, promotion)
	memory.AddUserMessage(chatIDStr, message)

	response, err := p.Communicate(ctx)
	if err != nil {
		logging.Error("Response error: %v", err)
		return nil, err
	}

	result, err := parseIntention(utils.PrepareLLMJSON(response.Content))
	if err != nil {
		logging.Error("Failed to parse intention: %v", err)
		return nil, err
	}
	result.Goal = userMessage
	return result, nil

}

// ShouldReclassifyIntent 在已有意图时判断是否需要再次调用 ConfirmIntention。
// 用规则过滤掉大部分消息，只在「明显可能换模式」时才走 LLM，避免每条消息都分类。
func ShouldReclassifyIntent(currentIntent, userMessage string) bool {
	cur := strings.ToUpper(strings.TrimSpace(currentIntent))
	msg := strings.TrimSpace(userMessage)
	if msg == "" {
		return false
	}
	low := strings.ToLower(msg)
	runes := utf8.RuneCountInString(msg)

	// 显式切换 / 重置（与 intention prompt 中 SWITCH 描述对齐）
	switchHints := []string{
		"切换到", "换模式", "换个模式", "重置意图", "重新确认", "重新分类",
		"switch to", "change to", "change mode", "reset intent", "new mode",
	}
	for _, h := range switchHints {
		if strings.Contains(low, h) {
			return true
		}
	}

	switch cur {
	case utils.PlanModeString:
		// 仍在规划/长任务中，但新输入很短且像单次工具或闲聊
		if runes <= 120 && messageLooksLikeAtomicToolOrChat(low) {
			return true
		}
	case utils.ToolModeString, utils.ChatModeString:
		// 当前是轻量模式，但新输入明显像多步任务
		if runes >= 40 && messageLooksLikeMultiStepPlan(low) {
			return true
		}
	}
	return false
}

func messageLooksLikeAtomicToolOrChat(low string) bool {
	hints := []string{
		"天气", "气温", "weather", "温度",
		"价格", "股价", "行情", "bitcoin", "btc", "汇率",
		"翻译", "translate",
		"几点", "现在时间", "当前时间", "时间", "timezone",
		"搜索", "查一下", "查询", "google",
		// 在 PLAN 模式下：明确表示先聊天/不跑执行，便于重判为 CHAT/TOOL
		"闲聊", "聊聊天", "先别执行", "不要执行", "仅讨论", "只讨论", "先讨论",
		"switch to chat", "chat only", "no execution",
	}
	for _, h := range hints {
		if strings.Contains(low, h) {
			return true
		}
	}
	return false
}

func messageLooksLikeMultiStepPlan(low string) bool {
	hints := []string{
		"计划", "规划", "步骤", "分步", "多步", "阶段", "里程碑",
		"帮我做", "帮我实现", "实现一个", "开发一个", "搭建",
		"workflow", "roadmap", "step by step", "multi-step",
	}
	for _, h := range hints {
		if strings.Contains(low, h) {
			return true
		}
	}
	return false
}

func parseIntention(data string) (*Intention, error) {
	var i Intention

	err := json.Unmarshal([]byte(data), &i)
	if err != nil {
		return nil, err
	}

	if i.Intent == "" {
		return nil, errors.New("intent is empty")
	}

	if i.Intent != utils.ChatModeString && i.Intent != utils.ToolModeString && i.Intent != utils.PlanModeString && i.Intent != utils.SwitchModeString {
		return nil, errors.New("invalid intent type")
	}

	return &i, nil
}
