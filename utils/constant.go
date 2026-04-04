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

	FinishString     = "[DONE940720]"
	IntentKey        = "intentkey"
	ChatModeString   = "CHAT"
	PlanModeString   = "PLAN"
	ToolModeString   = "TOOL"
	SwitchModeString = "SWITCH"

	taskFailed     = "failed"
	taskCompleted  = "completed"
	taskInProgress = "inProgress"

	stepFailed    = "failed"
	stepCompleted = "completed"
	stepPending   = "pending"
	stepRunning   = "running"

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

	SingleToolPromptTemplate = `You are an intelligent assistant capable of help me with useing single tool.`
)

var (
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
