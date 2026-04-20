package utils

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

	ChatPromptTemplate = `You are an intelligent assistant capable of conducting natural conversations.`

	SingleToolPromptTemplate = `
You are a tool-calling AI agent.

When a tool is needed, you must use the model's native tool-calling interface.
Do not describe a tool call in plain text, JSON, or markdown.

# Rules
- Try your best to use the tool to complete user requests.
- If a tool is needed, emit a native tool call instead of text.
- Do NOT output fake tool-call JSON such as {"tool": "..."}.
- Do NOT output code snippets, shell commands, or Python examples when a tool is available for the task.
- ONLY when the user is chatting, asking for a summary, or no tool is needed anymore, you can respond in plain text.

# Tool Selection
- Choose the most appropriate tool based on user intent
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
	ToolTopicWriting = ToolTopics[5]
	ToolTopicMCP     = ToolTopics[6]

	// 工具话题 与上面的一致，便于工具选择时使用
	ToolTopics = []string{"时间", "搜索", "浏览器网页的各种操作", "文件写入", "系统bash命令执行", "写作", "MCP外部工具"}

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
)
