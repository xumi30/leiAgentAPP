package memory

import (
	"leiAgent/logging"
	"leiAgent/utils"
	"sync"
)

type MessageRole string

const (
	// MessageRoleUser represents a user message
	MessageRoleUser MessageRole = "user"
	// MessageRoleAssistant represents an assistant message
	MessageRoleAssistant MessageRole = "assistant"
	// MessageRoleSystem represents a system message
	MessageRoleSystem MessageRole = "system"
	// MessageRoleTool represents a tool response message
	MessageRoleTool MessageRole = "tool"
)

type Message struct {
	Role       MessageRole `json:"role"`
	Content    string      `json:"content,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
	Index    int              `json:"index"`
}

type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type localMemory struct {
	Messages map[string][]*Message
	RwLock   sync.RWMutex
}

var (
	localMemoryInstance *localMemory
	localMemoryOnce     sync.Once
)

// GetLocalMemory 获取全局单例 LocalMemory 对象
func GetLocalMemory() *localMemory {
	localMemoryOnce.Do(func() {
		localMemoryInstance = newLocalMemory()
	})
	return localMemoryInstance
}

func (m *localMemory) SetSystemPrompt(chatID string, systemPrompt string) {
	logging.Info("SetSystemPrompt called")
	m.RwLock.Lock()
	defer m.RwLock.Unlock()
	if _, ok := m.Messages[chatID]; !ok {
		m.Messages[chatID] = []*Message{}
	}
	m.Messages[chatID] = append(m.Messages[chatID], &Message{
		Role:    MessageRoleSystem,
		Content: systemPrompt,
	})
}

func newLocalMemory() *localMemory {
	messages := make(map[string][]*Message)
	return &localMemory{
		Messages: messages,
	}
}

func (m *localMemory) AddMessage(chatID string, message *Message) {
	// logging.Info("AddMessage called for chatID: %s: %v", chatID, message)
	m.RwLock.Lock()
	defer m.RwLock.Unlock()
	if _, ok := m.Messages[chatID]; !ok {
		m.Messages[chatID] = []*Message{}
	}
	m.Messages[chatID] = append(m.Messages[chatID], message)
}

func (m *localMemory) GetMessages(chatID string) []*Message {
	logging.Info("GetMessages called for chatID: %s", chatID)
	m.RwLock.RLock()
	defer m.RwLock.RUnlock()
	if msgs, ok := m.Messages[chatID]; ok {
		logging.Info("Messages found for chatID: %s", chatID)
		return msgs
	}
	return []*Message{}
}

func (m *localMemory) Clear(chatID string) {
	m.RwLock.Lock()
	defer m.RwLock.Unlock()
	delete(m.Messages, chatID)
}

func AddToolMessage(chatId, toolid string, toolMessage string) {
	if utils.IsBlank(toolMessage) && utils.IsBlank(toolid) {
		return
	}
	memoryLocal := GetLocalMemory()
	toolResultMsg := Message{
		Role:       MessageRoleTool,
		ToolCallID: toolid,
		Content:    toolMessage,
	}
	memoryLocal.AddMessage(chatId, &toolResultMsg)
}

func AddUserMessage(chatId, userMessage string) {
	if utils.IsBlank(userMessage) {
		return
	}
	memoryLocal := GetLocalMemory()
	userMsg := Message{
		Role:    MessageRoleUser,
		Content: userMessage,
	}
	memoryLocal.AddMessage(chatId, &userMsg)
}

func AddAssistantToolCallsMessage(chatId string, toolCalls []ToolCall) {
	if len(toolCalls) == 0 {
		return
	}
	memoryLocal := GetLocalMemory()
	assistantMsg := Message{
		Role:      MessageRoleAssistant,
		ToolCalls: toolCalls,
	}
	memoryLocal.AddMessage(chatId, &assistantMsg)
}

func AddAssistantContentMessage(chatId string, assistantMessage string) {
	if utils.IsBlank(assistantMessage) {
		return
	}
	memoryLocal := GetLocalMemory()
	assistantMsg := Message{
		Role:    MessageRoleAssistant,
		Content: assistantMessage,
	}
	memoryLocal.AddMessage(chatId, &assistantMsg)
}

func SetSystemPrompt(chatId string, systemprompt string) {
	if utils.IsBlank(systemprompt) {
		return
	}
	memoryLocal := GetLocalMemory()
	systempromptMsg := Message{
		Role:    MessageRoleSystem,
		Content: systemprompt,
	}
	memoryLocal.AddMessage(chatId, &systempromptMsg)
}
