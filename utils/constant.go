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
	ChatIDString     = "chatID"
	Channelstring    = "channel"
	Clientstring     = "httpClient"
	IsStreamString   = "IsStreamString"
	ToolsString      = "tools"
	IsPlanningString = "isPlanning"
	ToolTopicToLoad  = "toolTopicToLoad"

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

You usually call exactly ONE tool to respond to the user.

# Rules
- Respond with a valid tool call, except when user asks you to chat,summarize, or there is no need to call a tool anymore, then respond with a natural language.

# Tool Selection
- Choose the most appropriate tool based on user intent
- Do NOT guess missing parameters
- If required information is missing, call "final_answer" and ask for clarification

# Output Requirements
- Return ONLY a valid tool call
- Do NOT include any natural language outside the tool call

`
)

var (
	ToolTopicTime    = ToolTopics[0]
	ToolTopicSearch  = ToolTopics[1]
	ToolTopicBrowser = ToolTopics[2]
	ToolTopicFiles   = ToolTopics[3]
	ToolTopicSystem  = ToolTopics[4]
	ToolTopicWriting = ToolTopics[5]

	// 工具话题 与上面的一致，便于工具选择时使用
	ToolTopics = []string{"时间", "搜索", "浏览器网页的各种操作", "文件写入", "系统bash命令执行", "写作"}

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
