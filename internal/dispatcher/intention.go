package dispatcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"leiAgent/internal/memory"
	"leiAgent/internal/proxy"
	"leiAgent/logging"
	"leiAgent/utils"
	"strings"
	"unicode/utf8"
)

var promotion = fmt.Sprintf(`You are an intent classification module in an AI agent system.

CRITICAL OUTPUT RULE: Reply with exactly ONE JSON object and nothing else. Start with { and end with }. No markdown, no code fences, no reasoning before or after the JSON.

Your tasks:
1. Classify the user's PRIMARY intent into exactly ONE of: PLAN, TOOL, CHAT
2. Determine whether the request contains MULTIPLE independent user goals
3. ONLY if multiple independent goals exist → create sub_intents
4. Otherwise → sub_intents MUST be []
5. IF need more information or the request is ambiguous → requires_clarification = true, MUST ask the user for clarification in the "content" field

---

# Core Definitions

## maingoal
- A concise, high-level summary of the user's overall purpose
- Represents ONE unified goal

## sub_intents (IMPORTANT)
- Represent MULTIPLE independent user goals (NOT steps)
- Each goal must be completable independently
- MUST NOT be sequential steps or decomposition of maingoal

---

# Critical Rule (Stability)

Before creating sub_intents, you MUST decide:

Is this:
A) One goal with multiple steps → DO NOT create sub_intents  
B) Multiple independent goals → create sub_intents  

---

# INVALID CASE (MUST NOT DO)

User: "recommend travel places based on weather"

❌ WRONG:
sub_intents:
- "check weather"
- "recommend places"

Reason: these are steps, not independent goals

✔ CORRECT:
sub_intents = []

---

# Intent Definitions

## PLAN
- Requires multi-step reasoning or workflow
- Cannot be solved by a single tool call

## TOOL  
- Can be completed by a single tool call

## CHAT
- Informational or conversational
- No tool needed
- Put the user-facing reply (if any) inside the JSON string field "content" only — the overall model output must still be the JSON object, not plain prose.
---

# Primary Intent Rules

- If ANY part requires planning → PLAN
- Else if ANY part requires a tool → TOOL
- Else → CHAT

---

# Content Rules

- "content" is ONLY for direct user-facing answer
- DO NOT include reasoning or planning
- If nothing can be answered directly → ""

---

# TOOL Rules

If intent = TOOL:
- tooltopic MUST be one of [%s]

---

# Clarification

- If request is ambiguous → requires_clarification = true

---

# Output Format (STRICT JSON ONLY)

{
  "maingoal": "string",
  "intent": "PLAN | TOOL | CHAT",
  "confidence": 0.0-1.0,
  "reason": "short explanation",
  "requires_clarification": true | false,
  "content": "",
  "tooltopic": "optional",
  "sub_intents": [
    {
      "intent": "PLAN | TOOL | CHAT",
      "goal": "string",
      "content": "",
      "priority": "high | medium | low",
      "tooltopic": "optional"
    }
  ]
}

---

# Final Constraints

- sub_intents MUST be [] if only one goal
- NEVER break a single goal into steps
- Prefer TOOL over PLAN if possible
- Prefer CHAT if no execution is needed
- DO NOT output anything other than JSON
`, strings.Join(utils.ToolTopics, ", "))

type Intention struct {
	Goal                  string  `json:"maingoal"`
	Intent                string  `json:"intent"` // PLAN | TOOL | CHAT
	Confidence            float64 `json:"confidence"`
	Reason                string  `json:"reason"`
	RequiresClarification bool    `json:"requires_clarification"`

	Content   string `json:"content"`             // 直接回复用户的内容（仅用于简单场景）
	ToolTopic string `json:"tooltopic,omitempty"` // TOOL 专用

	SubIntents []SubIntent `json:"sub_intents,omitempty"` //
}

type SubIntent struct {
	Goal      string `json:"goal"`
	Intent    string `json:"intent"` // PLAN | TOOL | CHAT
	Content   string `json:"content"`
	ToolTopic string `json:"tooltopic,omitempty"` // TOOL 专用
}

func ConfirmIntention(ctx context.Context, message string) (*Intention, error) {

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
	// 意图分类只应看到 system+本轮 user，不要混入会话里的历史 assistant，否则模型易输出自然语言导致 JSON 解析失败。
	override := []*memory.Message{
		{Role: memory.MessageRoleSystem, Content: promotion},
		{Role: memory.MessageRoleUser, Content: message},
	}
	reqCtx := ctx
	reqCtx = context.WithValue(reqCtx, utils.MemoryMessagesOverrideString, override)
	reqCtx = context.WithValue(reqCtx, utils.IsStreamString, false)
	reqCtx = context.WithValue(reqCtx, utils.SkipDialogToUIString, true)

	response, err := p.Communicate(reqCtx)
	if err != nil {
		logging.Error("Response error: %v", err)
		return nil, err
	}

	result, err := parseIntention(utils.PrepareLLMJSON(response.Content))
	if err != nil {
		logging.Error("Failed to parse intention: %v", err)
		return nil, err
	}

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
