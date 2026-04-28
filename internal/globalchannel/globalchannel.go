package globalchannel

import (
	"context"
	"strings"

	"leiAgent/logging"
	"leiAgent/utils"
	"sync"
	"time"
)

// dialogOutChatIDFromCtx 优先使用 DialogOutChatID（子会话/临时 ctx 与 UI 会话分离时），否则回落 ChatIDString。
func dialogOutChatIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(utils.DialogOutChatIDString).(string); ok {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	if v, ok := ctx.Value(utils.ChatIDString).(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// agentIDFromCtx 从上下文中获取 AgentID
func agentIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(utils.AgentID).(string); ok {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

// ChannelConfig 配置选项
type ChannelConfig struct {
	BufferSize      int           // channel 缓冲区大小
	CleanupInterval time.Duration // 清理间隔
	MaxIdleTime     time.Duration // 最大空闲时间
}

// ChannelInfo channel 信息
type ChannelInfo struct {
	Channel    chan *Message
	LastAccess time.Time
	IsClosed   bool
}

// ChannelManager channel 管理器
type ChannelManager struct {
	channels    map[string]map[string]*ChannelInfo // chatID -> topic -> channelInfo
	mutex       sync.RWMutex
	config      ChannelConfig
	stopCleanup chan struct{}
}

// 默认配置
var defaultConfig = ChannelConfig{
	BufferSize:      100,
	CleanupInterval: 5 * time.Minute,
	MaxIdleTime:     30 * time.Minute,
}

type Message struct {
	ChatID          string   `json:"chatId,omitempty"`
	FromAgentID     string   `json:"fromAgentId,omitempty"`
	MessageID       string   `json:"messageId,omitempty"`
	Content         string   `json:"content,omitempty"`
	Role            string   `json:"role,omitempty"`
	IsFinished      bool     `json:"isFinished,omitempty"`
	TotalTokens     int      `json:"totalTokens,omitempty"`
	UserToAgentList []string `json:"userToAgentList,omitempty"`
	IsAutoToTalk    bool     `json:"isAutoToTalk,omitempty"`
	NeedNewChatName bool     `json:"needNewChatName,omitempty"`
}

// 全局单例实例
var globalManager *ChannelManager

// 初始化全局管理器
func init() {
	globalManager = NewChannelManager(defaultConfig)
	go globalManager.startCleanup()
}

// NewChannelManager 创建新的 channel 管理器
func NewChannelManager(config ChannelConfig) *ChannelManager {
	return &ChannelManager{
		channels:    make(map[string]map[string]*ChannelInfo),
		config:      config,
		stopCleanup: make(chan struct{}),
	}
}

// GetChannel 获取指定chatID和topic的channel，不存在时返回默认channel
func GetChannel(chatID, topic string) (chan *Message, error) {
	return globalManager.GetChannel(chatID, topic)
}

// RegisterChannel 注册或获取指定chatID和topic的channel
func RegisterChannel(chatID, topic string) (chan *Message, error) {
	return globalManager.RegisterChannel(chatID, topic)
}

// Cleanup 清理指定chatID的所有channel
func Cleanup(chatID string) error {
	return globalManager.Cleanup(chatID)
}

// GetChannelSize 获取当前管理的channel数量（调试用）
func GetChannelSize() int {
	return globalManager.GetChannelSize()
}

// GetChannel 获取指定chatID和topic的channel
func (m *ChannelManager) GetChannel(chatID, topic string) (chan *Message, error) {
	if chatID == "" || topic == "" {
		return nil, nil
	}

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if topicMap, exists := m.channels[chatID]; exists {
		if info, ok := topicMap[topic]; ok && !info.IsClosed {
			info.LastAccess = time.Now()
			return info.Channel, nil
		}
	}
	return nil, nil
}

// RegisterChannel 注册或获取指定chatID和topic的channel
func (m *ChannelManager) RegisterChannel(chatID, topic string) (chan *Message, error) {
	if chatID == "" || topic == "" {
		return nil, nil
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 检查是否已存在
	if topicMap, exists := m.channels[chatID]; exists {
		if info, ok := topicMap[topic]; ok && !info.IsClosed {
			info.LastAccess = time.Now()
			return info.Channel, nil
		}
	} else {
		// 创建chatID对应的map
		m.channels[chatID] = make(map[string]*ChannelInfo)
	}

	// 创建新的channel
	info := &ChannelInfo{
		Channel:    make(chan *Message, m.config.BufferSize),
		LastAccess: time.Now(),
		IsClosed:   false,
	}
	m.channels[chatID][topic] = info
	logging.Info("RegisterChannel: %s %s", chatID, topic)
	return info.Channel, nil
}

// Cleanup 清理指定chatID的所有channel
func (m *ChannelManager) Cleanup(chatID string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if topicMap, exists := m.channels[chatID]; exists {
		// 安全关闭所有channel
		for topic, info := range topicMap {
			if !info.IsClosed {
				func() {
					defer func() {
						if r := recover(); r != nil {
							logging.Error("Panic when closing channel %s: %v", topic, r)
						}
					}()
					close(info.Channel)
					info.IsClosed = true
				}()
			}
		}
		// 从map中移除
		delete(m.channels, chatID)
		logging.Info("Cleanup: %s", chatID)
	}
	return nil
}

// GetChannelSize 获取当前管理的channel数量
func (m *ChannelManager) GetChannelSize() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	count := 0
	for _, topicMap := range m.channels {
		count += len(topicMap)
	}
	return count
}

// startCleanup 启动定期清理
func (m *ChannelManager) startCleanup() {
	ticker := time.NewTicker(m.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.cleanupIdleChannels()
		case <-m.stopCleanup:
			return
		}
	}
}

// cleanupIdleChannels 清理空闲的channel
func (m *ChannelManager) cleanupIdleChannels() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	now := time.Now()
	for chatID, topicMap := range m.channels {
		for topic, info := range topicMap {
			if now.Sub(info.LastAccess) > m.config.MaxIdleTime && !info.IsClosed {
				logging.Info("Cleaning up idle channel: %s %s", chatID, topic)
				func() {
					defer func() {
						if r := recover(); r != nil {
							logging.Error("Panic when closing channel %s: %v", topic, r)
						}
					}()
					close(info.Channel)
					info.IsClosed = true
				}()
				delete(topicMap, topic)
			}
		}
		// 如果topicMap为空，删除chatID
		if len(topicMap) == 0 {
			delete(m.channels, chatID)
		}
	}
}

// Shutdown 关闭管理器
func Shutdown() {
	if globalManager != nil {
		close(globalManager.stopCleanup)
		// 清理所有channel
		globalManager.mutex.Lock()
		for chatID := range globalManager.channels {
			globalManager.Cleanup(chatID)
		}
		globalManager.mutex.Unlock()
	}
}

// 向后兼容的旧接口（避免破坏现有代码）
func GetGlobalChannel(chatID, topic string) chan *Message {
	ch, _ := GetChannel(chatID, topic)
	return ch
}

func RegisterGlobalChannel(chatID, topic string) chan *Message {
	ch, _ := RegisterChannel(chatID, topic)
	return ch
}

func RegisterGlobalInputChannel(chatID string) chan *Message {
	ch, _ := RegisterChannel(chatID, "inputchannel")
	logging.Info("RegisterGlobalInputChannel: %s", chatID)
	return ch
}

func RegisterGlobalDialogOutChannel(chatID string) chan *Message {
	ch, _ := RegisterChannel(chatID, "DialogOut")
	return ch
}

func RegisterGlobalReasonOutChannel(chatID string) chan *Message {
	ch, _ := RegisterChannel(chatID, "ReasonOut")
	return ch
}

func RegisterGlobalTaskStateChannel(chatID string) chan *Message {
	ch, _ := RegisterChannel(chatID, "TaskState")
	return ch
}

func GetGlobalInputChannel(chatID string) chan *Message {
	ch, _ := GetChannel(chatID, "inputchannel")
	logging.Info("GetGlobalInputChannel: %s", chatID)
	return ch
}

func GetGlobalDialogOutChannel(chatID string) chan *Message {
	ch, _ := GetChannel(chatID, "DialogOut")
	return ch
}

func GetGlobalReasonOutChannel(chatID string) chan *Message {
	ch, _ := GetChannel(chatID, "ReasonOut")
	return ch
}

func GetGlobalTaskStateChannel(chatID string) chan *Message {
	ch, _ := GetChannel(chatID, "TaskState")
	return ch
}

func CleanupGlobalChannel(chatID string) {
	_ = Cleanup(chatID)
}

func SendAssitantMessageOnce(ctx context.Context, msg string, totalTokens ...int) {
	chatID := dialogOutChatIDFromCtx(ctx)
	dialogOutChan := GetGlobalDialogOutChannel(chatID)
	messageid := utils.GenerateMessageID()
	tok := 0
	if len(totalTokens) > 0 {
		tok = totalTokens[0]
	}
	mg := Message{
		FromAgentID: agentIDFromCtx(ctx),
		MessageID:   messageid,
		Content:     msg,
		Role:        utils.MessageRoleAssistant,
		IsFinished:  true,
		TotalTokens: tok,
	}

	if dialogOutChan == nil {
		logging.Warn("SendAssitantMessageOnce: DialogOut channel 未注册 chatID=%q（消息将丢弃）", chatID)
		return
	}
	dialogOutChan <- &mg

}

func SendUserMessageOnce(ctx context.Context, msg string) {
	chatID := dialogOutChatIDFromCtx(ctx)
	dialogOutChan := GetGlobalDialogOutChannel(chatID)
	messageid := utils.GenerateMessageID()

	if dialogOutChan == nil {
		logging.Warn("SendUserMessageOnce: DialogOut channel 未注册 chatID=%q", chatID)
		return
	}
	dialogOutChan <- &Message{
		FromAgentID: agentIDFromCtx(ctx),
		MessageID:   messageid,
		Content:     msg,
		Role:        utils.MessageRoleUser,
		IsFinished:  true,
	}

}

func SendAssitantMessageStream(ctx context.Context, msg string, messageid string, isFinish bool, totalTokens int) {
	chatID := dialogOutChatIDFromCtx(ctx)
	dialogOutChan := GetGlobalDialogOutChannel(chatID)
	if dialogOutChan == nil {
		logging.Warn("SendAssitantMessageStream: DialogOut channel 未注册 chatID=%q finish=%v（丢弃）", chatID, isFinish)
		return
	}

	dialogOutChan <- &Message{
		FromAgentID: agentIDFromCtx(ctx),
		MessageID:   messageid,
		Content:     msg,
		Role:        utils.MessageRoleAssistant,
		IsFinished:  isFinish,
		TotalTokens: totalTokens,
	}

}

func SendAReasonningMessageStream(ctx context.Context, msg string, messageid string, isFinish bool) {
	chatID := dialogOutChatIDFromCtx(ctx)
	reasonOutChan := GetGlobalReasonOutChannel(chatID)
	if reasonOutChan == nil {
		logging.Warn("SendAReasonningMessageStream: ReasonOut channel 未注册 chatID=%q finish=%v", chatID, isFinish)
		return
	}

	reasonOutChan <- &Message{
		FromAgentID: agentIDFromCtx(ctx),
		MessageID:   messageid,
		Content:     msg,
		Role:        utils.MessageRoleReasoning,
		IsFinished:  isFinish,
	}

}

func SendTaskState(ctx context.Context, busy bool) {
	chatID := ctx.Value(utils.ChatIDString).(string)
	taskStateChan := GetGlobalTaskStateChannel(chatID)
	if taskStateChan == nil {
		return
	}
	content := "idle"
	if busy {
		content = "busy"
	}
	taskStateChan <- &Message{
		FromAgentID: agentIDFromCtx(ctx),
		MessageID:   utils.GenerateMessageID(),
		Content:     content,
		Role:        "task_state",
		IsFinished:  !busy,
	}
}
