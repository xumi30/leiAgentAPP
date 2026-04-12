package memory

import (
	"fmt"
	"leiAgent/internal/tools"
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

	// assistantReplyTurns 记录每会话「助手正文回复」次数，用于每满 AutoCompressEveryAssistantTurns 触发记忆压缩。
	assistantReplyTurns map[string]int
	compressTurnMu      sync.Mutex
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

func (m *localMemory) SetSystemPrompt(chatID string, systemPrompt string) int {
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
	return len(m.Messages[chatID]) - 1
}

func (m *localMemory) ClearSystemPrompt(chatID string) {
	m.RwLock.Lock()
	defer m.RwLock.Unlock()
	if _, ok := m.Messages[chatID]; !ok {
		return
	}
	// 遍历找到role为system的message，如果存在，则删除，否则返回
	for i, msg := range m.Messages[chatID] {
		if msg.Role == MessageRoleSystem {
			deleted := append(m.Messages[chatID][:i], m.Messages[chatID][i+1:]...)
			m.Messages[chatID] = deleted
			return
		}
	}
	logging.Info("clear SystemPrompt called: %s", chatID)
}

func newLocalMemory() *localMemory {
	messages := make(map[string][]*Message)
	return &localMemory{
		Messages:            messages,
		assistantReplyTurns: make(map[string]int),
	}
}

func (m *localMemory) AddMessage(chatID string, message *Message) int {
	// logging.Info("AddMessage called for chatID: %s: %v", chatID, message)
	m.RwLock.Lock()
	defer m.RwLock.Unlock()
	if _, ok := m.Messages[chatID]; !ok {
		m.Messages[chatID] = []*Message{}
	}
	m.Messages[chatID] = append(m.Messages[chatID], message)
	return len(m.Messages[chatID]) - 1
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
	m.compressTurnMu.Lock()
	delete(m.assistantReplyTurns, chatID)
	m.compressTurnMu.Unlock()
}

func (m *localMemory) DeleteMemoryByIndex(chatID string, index int) bool {
	m.RwLock.Lock()
	defer m.RwLock.Unlock()
	if msgs, ok := m.Messages[chatID]; ok {
		if index >= 0 && index < len(msgs) {
			m.Messages[chatID][index] = nil
			return true
		}
	}
	return false
}

func AddToolMessage(chatId, toolid string, toolMessage string) int {
	if utils.IsBlank(toolMessage) && utils.IsBlank(toolid) {
		return -1
	}
	memoryLocal := GetLocalMemory()
	toolResultMsg := Message{
		Role:       MessageRoleTool,
		ToolCallID: toolid,
		Content:    toolMessage,
	}
	return memoryLocal.AddMessage(chatId, &toolResultMsg)
}

func AddUserMessage(chatId, userMessage string) int {
	if utils.IsBlank(userMessage) {
		return -1
	}
	memoryLocal := GetLocalMemory()
	userMsg := Message{
		Role:    MessageRoleUser,
		Content: userMessage,
	}
	return memoryLocal.AddMessage(chatId, &userMsg)
}

func AddAssistantToolCallsMessage(chatId string, toolCalls []ToolCall) int {
	if len(toolCalls) == 0 {
		return -1
	}
	memoryLocal := GetLocalMemory()
	assistantMsg := Message{
		Role:      MessageRoleAssistant,
		ToolCalls: toolCalls,
	}
	return memoryLocal.AddMessage(chatId, &assistantMsg)
}

func AddAssistantContentMessage(chatId string, assistantMessage string) int {
	if utils.IsBlank(assistantMessage) {
		return -1
	}
	memoryLocal := GetLocalMemory()
	assistantMsg := Message{
		Role:    MessageRoleAssistant,
		Content: assistantMessage,
	}
	idx := memoryLocal.AddMessage(chatId, &assistantMsg)
	memoryLocal.afterAssistantContentTurn(chatId)
	return idx
}

func SetSystemPrompt(chatId string, systemprompt string) int {
	if utils.IsBlank(systemprompt) {
		return -1
	}
	memoryLocal := GetLocalMemory()
	for i, msg := range memoryLocal.Messages[chatId] {
		if msg.Role == MessageRoleSystem {
			msg.Content = systemprompt
			logging.Info("update SystemPrompt called: %s", chatId)
			return i
		}
	}
	return memoryLocal.SetSystemPrompt(chatId, systemprompt)
}

func SetToolsInfo(chatId string) {
	toolRegistry := tools.Getregistry()
	js, err := toolRegistry.ConvertToolsToJSON()
	if err != nil {
		logging.Error("ConvertToolsToJSON failed: %v", err)

	}
	AddUserMessage(chatId, fmt.Sprintf("这些是你能使用的工具消息：\n%s", js))
}

func DeleteMemoryByIndex(chatId string, index int) bool {
	memoryLocal := GetLocalMemory()
	return memoryLocal.DeleteMemoryByIndex(chatId, index)
}
