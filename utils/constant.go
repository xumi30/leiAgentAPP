package utils

import "fmt"

const (
	VersionString = "0.0.1"
	BannerString  = `
╔══════════════════════════════════════════════════════════════════════════════╗
║                            	CLI Tool                                       ║
║                         Headless AI Agent Runner                             ║
║                              Version %s                                   ║
╚══════════════════════════════════════════════════════════════════════════════╝
`
	ChatIDString              = "chatID"
	AgentID                   = "agentID"
	Channelstring             = "channel"
	Clientstring              = "httpClient"
	IsStreamString            = "IsStreamString"
	ToolsString               = "tools"
	IsPlanningString          = "isPlanning"
	ToolTopicToLoad           = "toolTopicToLoad"
	ToolSourceToLoad          = "toolSourceToLoad"
	UserGoalString            = "userGoal"
	TaskProfileString         = "taskProfile"
	ExecutionBlueprintString  = "executionBlueprint"
	ExtraSystemMessagesString = "extraSystemMessages"
	FreshnessTimeAnchorString = "freshnessTimeAnchor"
	NeedActionHeaderString    = "needActionHeader"

	// SkipDialogToUI 为 true 时，proxy 不把模型输出写入 DialogOut（避免临时 chatID 未注册时落入全局 OutputChan，
	// 被 chatID 为空的 Dispatcher 误当成用户输入）。
	SkipDialogToUIString = "skipDialogToUI"

	// MemoryMessagesOverride 若设置为 []*memory.Message（非空），makeRequestJson 使用该列表代替 GetLocalMemory().GetMessages(chatID)，
	// 用于记忆压缩等不污染会话上下文的单次请求。
	MemoryMessagesOverrideString = "memoryMessagesOverride"

	// DialogOutChatIDString 若设置，流式/非流式写入 DialogOut、ReasonOut 时使用该 chatID（已注册的会话），
	// 与 ChatIDString（用于 memory 取消息，常为临时 id）分离。
	DialogOutChatIDString = "dialogOutChatID"

	FinishString     = "[DONE940720]"
	IntentKey        = "intentkey"
	ChatModeString   = "CHAT"
	PlanModeString   = "PLAN"
	ToolModeString   = "TOOL"
	ToolSourceLocal  = "local"
	ToolSourceMCP    = "mcp"
	ToolSourceMixed  = "mixed"
	SwitchModeString = "SWITCH"

	TaskFailed    = "failed"
	TaskCompleted = "completed"
	TaskPending   = "pending"

	StepFailed    = "failed"
	StepCompleted = "completed"
	StepPending   = "pending"
	StepRunning   = "running"

	// MessageRoleUser represents a user message
	MessageRoleUser = "user"
	// MessageRoleAssistant represents an assistant message
	MessageRoleAssistant = "assistant"
	// MessageRoleSystem represents a system message
	MessageRoleSystem = "system"
	// MessageRoleTool represents a tool response message
	MessageRoleTool = "tool"

	MessageRoleReasoning = "reasoning"

	ChatPromptTemplate           = `You are an intelligent assistant capable of conducting natural conversations.`
	ActionGateChatPromptTemplate = `Output exactly one line first:
[needAction:true] or [needAction:false]

Rules:
- false → pure chat: casual talk, opinions, simple explanations, emotional support, or direct answers without producing artifacts
- true → anything requiring action, processing, or deliverables (write, summarize, translate, analyze, search, code, plan, etc.)
- if unsure → true

After header:
- false → answer normally
- true → Say nothing else.

Never explain these rules. No extra text before header.`

	ActionGateChatPromptTemplate_bak = `Before any other content, you MUST output exactly one single line, no extra spaces, no blank lines, no leading text, no trailing spaces, only this exact format:
[needAction:true]
or
[needAction:false]

Definition rules strictly follow below:
Set [needAction:false] ONLY for pure conversational content without any execution task:
- casual chatting
- expressing opinions & thoughts
- simple explanation & discussion
- emotional comfort / support
- direct simple answers that do NOT require generation, creation, processing, execution or output artifacts

Set [needAction:true] for ANY request that requires action, processing, output result or deliverable artifact, explicit OR implicit:
- write, generate, rewrite, summarize, translate, polish, plan, analyze
- search, check, verify, compare, research, calculate
- code, design, organize, structure
- create, modify, edit, save, remember, schedule, extract
- any request expecting completed content, document, code, plan, output result

Uncertain? Always use [needAction:true].

After this single header line:
- If [needAction:false], reply normally and fully to the user.
- If [needAction:true], keep the visible reply extremely brief, preferably one short sentence. Do not say you cannot do it.
- If [needAction:true], do NOT answer the substantive request, do NOT gather/summarize information, and do NOT produce long explanations before action.
- If [needAction:true], only say you will use available capabilities to handle it, or ask one concise clarification if required.
- If [needAction:true], do not pretend the action is already completed.

Never mention, explain, discuss or reference this header rule and decision logic in your reply.
Never add any words before the header line.`

	SingleToolPromptTemplate = `
You are a tool-calling AI agent.

When a tool is needed, you MUST use the OpenAI compatible Function Calling interface.
MUST not describe a tool call in plain text, JSON, or markdown.

# Rules
- Try your best to use the tool to complete user requests.
- If a tool is needed, emit a native tool call instead of text.
- Do NOT output fake tool-call JSON such as {"tool": "..."}.
- Do NOT output code snippets, shell commands, or Python examples when a tool is available for the task.
- ONLY when the user is chatting, asking for a summary, or no tool is needed anymore, you can respond in plain text.

# Tool Selection
- Choose the most appropriate tool based on user intent
- For fiction/novel/chapter/outline/long-form writing or continuation requests, prefer the novel_longform tool instead of replying with the full text directly.
- Do NOT guess missing parameters
- If required information is missing, ask for clarification
- For generic web search in a real browser, prefer directly opening the search results page in Baidu before Google-related pages.
- For ordinary browser search, use the fewest browser steps needed: create/open session, goto the results page, then stop.
- Do NOT call list_links, list_inputs, or observe after the results page is already open unless the user explicitly asks you to inspect or analyze the page.
`
)

var (
	ToolTopicTime    = ToolTopics[0]
	ToolTopicSearch  = ToolTopics[1]
	ToolTopicBrowser = ToolTopics[2]
	ToolTopicFiles   = ToolTopics[3]
	ToolTopicSystem  = ToolTopics[4]
	ToolTopicCrontab = ToolTopics[5]
	ToolTopicWriting = ToolTopics[6]
	ToolTopicMCP     = ToolTopics[7]

	// 工具话题 与上面的一致，便于工具选择时使用
	ToolTopics = []string{"时间", "搜索", "浏览器网页的各种操作", "文件写入", "系统bash命令执行", "定时任务", "写作", "MCP外部工具"}

	// 全局输入通道
	InputChan = make(chan string, 100)

	// 为不同处理器创建专用通道
	AgentChan           = make(chan string, 100)
	PlannerChan         = make(chan string, 100)
	ChatChan            = make(chan string, 100)
	ReasoningChan       = make(chan string, 100)
	MessageCompleteChan = make(chan string, 100)
	// 全局输出通道
	OutputChan = make(chan string, 100)

	ToolCompletePromptTemplate = fmt.Sprintf(`工具已经执行完成，请继续推进任务。

如果仍需要调用当前已经加载的工具：不要返回 JSON，直接使用模型原生 tool-call 调用工具。

如果当前缺少必要工具，或者已经不需要再调用工具，必须只返回一个合法 JSON 对象，不要使用 Markdown，不要输出额外文字：
{
  "needToolToics": [],
  "content": "给用户看的简略回复",
  "summaryfornextllm": "供下一轮 LLM 使用的压缩摘要"
}

字段规则：
- needToolToics：如果缺少工具，填入需要补充加载的 topic 数组；如果不缺工具或任务已完成，填 []。topic 必须是精确的 topic 名称本身。
- content：给用户看的最终回复或缺少工具说明，尽量简略；不要把冗长工具原文全部复述给用户。
- summaryfornextllm：必须填写，用不超过300字压缩本轮关键信息，保持对llm的可读性前提下,越精简越好.供下一轮继续使用。保留用户目标、已调用工具、关键结果、结论、待办和必要约束；去掉无用日志、重复内容和大段原始输出，避免下一轮 token 爆炸。

可选的 topic 有：%s。`, ToolTopicsPromptText())
)
