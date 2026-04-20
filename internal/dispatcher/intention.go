package dispatcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"leiAgent/internal/memory"
	"leiAgent/internal/provider/openaistyle"
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
2. If intent = TOOL, choose tool_source from: local, mcp, mixed
3. Determine whether the request contains MULTIPLE independent user goals
4. ONLY if multiple independent goals exist → create sub_intents
5. Otherwise → sub_intents MUST be []
6. IF need more information or the request is ambiguous → requires_clarification = true, MUST ask the user for clarification in the "content" field
7. Use the provided recent_context and current_state as lightweight conversation context when the latest user message depends on previous turns

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
- Requires explicit decomposition, staged execution, or long-horizon coordination
- The user is asking the agent to plan or carry out a multi-phase task
- Do NOT choose PLAN just because some light reasoning is helpful

## TOOL  
- The request is primarily about using tools or external capabilities to get/do something
- Tool use may involve one or a small number of tightly related tool actions
- Choose TOOL when the task is operational and does not need explicit project-style planning

## CHAT
- Informational or conversational
- No tool needed
---

# Primary Intent Rules

- Prefer the LIGHTEST sufficient intent
- Choose PLAN only when explicit planning or staged execution is necessary
- Else if tools are needed to fulfill the request → TOOL
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
- tool_source MUST be one of: local, mcp, mixed
- If no topic matches cleanly, choose the closest topic instead of inventing a new one
- Prefer mcp when MCP_LIST shows relevant capability that overlaps with local tools
- Use local only when local tools are clearly more appropriate or no suitable MCP capability exists

---

# Clarification

- If request is ambiguous → requires_clarification = true
- If requires_clarification = true, content MUST contain a short user-facing clarification question
- If requires_clarification = true, still choose the best current intent

---

# Output Format (STRICT JSON ONLY)

{
  "maingoal": "string",
  "intent": "PLAN | TOOL | CHAT",
  "confidence": 0.0-1.0,
  "reason": "short explanation",
  "requires_clarification": true | false,
  "content": "",
  "tool_source": "local | mcp | mixed",
  "tooltopic": "optional",
  "sub_intents": [
    {
      "intent": "PLAN | TOOL | CHAT",
      "goal": "string",
      "content": "",
      "tool_source": "local | mcp | mixed",
      "priority": "high | medium | low",
      "tooltopic": "optional"
    }
  ]
}

---

# Final Constraints

- sub_intents MUST be [] if only one goal
- NEVER break a single goal into steps
- Prefer the least complex valid intent
- Prefer TOOL over PLAN when no explicit staged execution is required
- Prefer CHAT if no execution is needed
- DO NOT output anything other than JSON
`, utils.ToolTopicsPromptText())

type Intention struct {
	Goal                  string  `json:"maingoal"`
	Intent                string  `json:"intent"` // PLAN | TOOL | CHAT
	Confidence            float64 `json:"confidence"`
	Reason                string  `json:"reason"`
	RequiresClarification bool    `json:"requires_clarification"`

	Content    string `json:"content"`               // 直接回复用户的内容（仅用于简单场景）
	ToolTopic  string `json:"tooltopic,omitempty"`   // TOOL 专用
	ToolSource string `json:"tool_source,omitempty"` // TOOL 来源: local | mcp | mixed

	SubIntents []SubIntent `json:"sub_intents,omitempty"` //
}

type SubIntent struct {
	Goal       string `json:"goal"`
	Intent     string `json:"intent"` // PLAN | TOOL | CHAT
	Content    string `json:"content"`
	ToolTopic  string `json:"tooltopic,omitempty"`   // TOOL 专用
	ToolSource string `json:"tool_source,omitempty"` // TOOL 来源
	Priority   string `json:"priority,omitempty"`
}

type IntentContextMessage struct {
	Role    string `json:"role"`
	Content string `json:"content,omitempty"`
}

type IntentCurrentState struct {
	Intent     string `json:"intent,omitempty"`
	ToolTopic  string `json:"tooltopic,omitempty"`
	ToolSource string `json:"tool_source,omitempty"`
}

type IntentInput struct {
	CurrentMessage string                 `json:"current_message"`
	RecentContext  []IntentContextMessage `json:"recent_context"`
	CurrentState   IntentCurrentState     `json:"current_state,omitempty"`
	Tools          interface{}            `json:"TOOL_LIST"`
	MCPs           interface{}            `json:"MCP_LIST"`
}

func ConfirmIntention(ctx context.Context, message string, currentState *Intention) (*Intention, error) {

	js := getToolsSimpleInfo()
	intentInput := IntentInput{
		CurrentMessage: message,
		RecentContext:  buildIntentRecentContext(ctx, message),
		Tools:          json.RawMessage(js),
		MCPs:           json.RawMessage(getMCPSimpleInfo()),
	}
	if currentState != nil {
		intentInput.CurrentState = IntentCurrentState{
			Intent:     strings.TrimSpace(currentState.Intent),
			ToolTopic:  strings.TrimSpace(currentState.ToolTopic),
			ToolSource: strings.TrimSpace(currentState.ToolSource),
		}
	}
	intentJSON, err := json.MarshalIndent(intentInput, "", "  ")
	if err != nil {
		logging.Error("序列化规划输入失败: %v", err)
	}

	message = `这是意图识别输入。current_message 是当前用户输入，recent_context 是最近几条真实对话上下文，current_state 是当前会话已知状态，TOOL_LIST 是本地工具概览，MCP_LIST 是已连接 MCP 能力概览。请基于这些信息判断用户请求意图:` + string(intentJSON)

	chatIDVal := ctx.Value(utils.ChatIDString)
	chatIDStr, ok := chatIDVal.(string)
	if !ok || strings.TrimSpace(chatIDStr) == "" {
		return nil, errors.New("ConfirmIntention: context 缺少有效的 chatID")
	}

	p, err := proxy.NewProxy(nil)
	if err != nil {
		return nil, err
	}

	ctx = context.WithValue(ctx, utils.IsStreamString, false)
	ctx = context.WithValue(ctx, utils.SkipDialogToUIString, true)
	ctx = context.WithValue(ctx, utils.DialogOutChatIDString, chatIDStr)

	response, err := p.CommunicateWithMessages(ctx, []openaistyle.ChatMessage{
		{
			Role:    openaistyle.RoleSystem,
			Content: promotion,
		},
		{
			Role:    openaistyle.RoleUser,
			Content: message,
		},
	})
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

func buildIntentRecentContext(ctx context.Context, currentMessage string) []IntentContextMessage {
	chatIDVal := ctx.Value(utils.ChatIDString)
	chatID, ok := chatIDVal.(string)
	if !ok || strings.TrimSpace(chatID) == "" {
		return nil
	}

	msgs := memory.GetLocalMemory().GetMessages(chatID)
	if len(msgs) == 0 {
		return nil
	}

	contextMessages := make([]IntentContextMessage, 0, 6)
	currentTrimmed := strings.TrimSpace(currentMessage)
	for i := len(msgs) - 1; i >= 0 && len(contextMessages) < 6; i-- {
		msg := msgs[i]
		if msg == nil {
			continue
		}
		if msg.Role == memory.MessageRoleSystem {
			continue
		}

		content := normalizeIntentContextContent(msg.Content)
		if strings.TrimSpace(content) == "" {
			if len(msg.ToolCalls) == 0 && msg.ToolCallID == "" {
				continue
			}
			content = summarizeToolMessage(msg)
		}

		if strings.TrimSpace(content) == "" {
			continue
		}
		if currentTrimmed != "" && strings.TrimSpace(content) == currentTrimmed {
			continue
		}

		contextMessages = append(contextMessages, IntentContextMessage{
			Role:    string(msg.Role),
			Content: utils.TruncateRunes(content, 280),
		})
	}

	reverseIntentContextMessages(contextMessages)
	return contextMessages
}

func normalizeIntentContextContent(content string) string {
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "用户请求:") {
		return strings.TrimSpace(strings.TrimPrefix(trimmed, "用户请求:"))
	}
	return trimmed
}

func summarizeToolMessage(msg *memory.Message) string {
	if msg == nil {
		return ""
	}
	if len(msg.ToolCalls) > 0 {
		names := make([]string, 0, len(msg.ToolCalls))
		for _, call := range msg.ToolCalls {
			name := strings.TrimSpace(call.Function.Name)
			if name != "" {
				names = append(names, name)
			}
		}
		if len(names) > 0 {
			return "工具调用: " + strings.Join(names, ", ")
		}
	}
	if strings.TrimSpace(msg.ToolCallID) != "" && strings.TrimSpace(msg.Content) != "" {
		return "工具结果: " + strings.TrimSpace(msg.Content)
	}
	return strings.TrimSpace(msg.Content)
}

func reverseIntentContextMessages(messages []IntentContextMessage) {
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
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

	i.Intent = strings.ToUpper(strings.TrimSpace(i.Intent))
	i.ToolTopic = strings.TrimSpace(i.ToolTopic)
	i.ToolSource = normalizeToolSource(i.ToolSource)
	i.Content = strings.TrimSpace(i.Content)
	i.Goal = strings.TrimSpace(i.Goal)

	for idx := range i.SubIntents {
		i.SubIntents[idx].Intent = strings.ToUpper(strings.TrimSpace(i.SubIntents[idx].Intent))
		i.SubIntents[idx].ToolTopic = strings.TrimSpace(i.SubIntents[idx].ToolTopic)
		i.SubIntents[idx].ToolSource = normalizeToolSource(i.SubIntents[idx].ToolSource)
		i.SubIntents[idx].Content = strings.TrimSpace(i.SubIntents[idx].Content)
		i.SubIntents[idx].Goal = strings.TrimSpace(i.SubIntents[idx].Goal)
		i.SubIntents[idx].Priority = strings.TrimSpace(i.SubIntents[idx].Priority)
	}

	if i.Intent == "" {
		return nil, errors.New("intent is empty")
	}

	if i.Intent != utils.ChatModeString && i.Intent != utils.ToolModeString && i.Intent != utils.PlanModeString && i.Intent != utils.SwitchModeString {
		return nil, errors.New("invalid intent type")
	}

	if i.RequiresClarification && i.Content == "" {
		return nil, errors.New("clarification requested but content is empty")
	}

	return &i, nil
}

func normalizeToolSource(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case utils.ToolSourceMCP:
		return utils.ToolSourceMCP
	case utils.ToolSourceMixed:
		return utils.ToolSourceMixed
	default:
		return utils.ToolSourceLocal
	}
}
