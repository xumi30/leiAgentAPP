package globalchannel

import (
	"leiAgent/logging"
	"leiAgent/utils"
	"sync"
	"time"
)

// ChannelConfig 配置选项
type ChannelConfig struct {
	BufferSize      int           // channel 缓冲区大小
	CleanupInterval time.Duration // 清理间隔
	MaxIdleTime     time.Duration // 最大空闲时间
}

// ChannelInfo channel 信息
type ChannelInfo struct {
	Channel    chan string
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
func GetChannel(chatID, topic string) (chan string, error) {
	return globalManager.GetChannel(chatID, topic)
}

// RegisterChannel 注册或获取指定chatID和topic的channel
func RegisterChannel(chatID, topic string) (chan string, error) {
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
func (m *ChannelManager) GetChannel(chatID, topic string) (chan string, error) {
	if chatID == "" || topic == "" {
		return utils.OutputChan, nil
	}

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if topicMap, exists := m.channels[chatID]; exists {
		if info, ok := topicMap[topic]; ok && !info.IsClosed {
			info.LastAccess = time.Now()
			logging.Info("GetChannel: %s %s", chatID, topic)
			return info.Channel, nil
		}
	}
	return utils.OutputChan, nil
}

// RegisterChannel 注册或获取指定chatID和topic的channel
func (m *ChannelManager) RegisterChannel(chatID, topic string) (chan string, error) {
	if chatID == "" || topic == "" {
		return utils.OutputChan, nil
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
		Channel:    make(chan string, m.config.BufferSize),
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
func GetGlobalChannel(chatID, topic string) chan string {
	ch, _ := GetChannel(chatID, topic)
	return ch
}

func RegisterGlobalChannel(chatID, topic string) chan string {
	ch, _ := RegisterChannel(chatID, topic)
	return ch
}

func RegisterGlobalInputChannel(chatID string) chan string {
	ch, _ := RegisterChannel(chatID, "inputchannel")
	logging.Info("RegisterGlobalInputChannel: %s", chatID)
	return ch
}

func RegisterGlobalDialogOutChannel(chatID string) chan string {
	ch, _ := RegisterChannel(chatID, "DialogOut")
	return ch
}

func RegisterGlobalReasonOutChannel(chatID string) chan string {
	ch, _ := RegisterChannel(chatID, "ReasonOut")
	return ch
}

func GetGlobalInputChannel(chatID string) chan string {
	ch, _ := GetChannel(chatID, "inputchannel")
	logging.Info("GetGlobalInputChannel: %s", chatID)
	return ch
}

func GetGlobalDialogOutChannel(chatID string) chan string {
	ch, _ := GetChannel(chatID, "DialogOut")
	return ch
}

func GetGlobalReasonOutChannel(chatID string) chan string {
	ch, _ := GetChannel(chatID, "ReasonOut")
	return ch
}

func CleanupGlobalChannel(chatID string) {
	_ = Cleanup(chatID)
}
