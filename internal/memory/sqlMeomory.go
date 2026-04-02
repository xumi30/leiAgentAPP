package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"leiAgent/logging"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// SQLMemory 使用 SQLite 持久化存储对话历史
type SQLMemory struct {
	db     *sql.DB
	dbPath string
	mu     sync.RWMutex
}

// 添加单例相关变量和函数
var (
	instance *SQLMemory
	once     sync.Once
)

// GetInstance 获取 SQLMemory 单例实例
func GetSqlInstance(dbPath string) (*SQLMemory, error) {
	var err error
	once.Do(func() {
		instance, err = NewSQLMemory(dbPath)
	})
	return instance, err
}

// GetExistingInstance 获取已存在的 SQLMemory 单例实例，如果不存在则返回 nil
func GetExistingInstance() *SQLMemory {
	return instance
}

// ResetInstance 重置单例实例（主要用于测试）
func ResetInstance() {
	instance = nil
	once = sync.Once{}
}

// NewSQLMemory 创建一个新的 SQLMemory 实例
func NewSQLMemory(dbPath string) (*SQLMemory, error) {
	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// 打开数据库连接
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// 创建表结构
	if err := createTables(db); err != nil {
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	return &SQLMemory{
		db:     db,
		dbPath: dbPath,
	}, nil
}

// createTables 创建数据库表结构
func createTables(db *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS conversations (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			mode TEXT DEFAULT 'chat'
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			messageID 	TEXT NOT NULL,
			chatID TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (chatID) REFERENCES conversations(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_chatID ON messages(chatID)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_timestamp ON messages(timestamp)`,
	}
	// role: user assistant system reasoning

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("failed to execute query: %w", err)
		}
	}

	return nil
}

// SaveConversation 保存或更新对话
func (m *SQLMemory) SaveConversation(chatID, title, mode string) error {

	//需要确保chatID是纯数字
	if _, err := strconv.Atoi(chatID); err != nil {
		logging.Error("conversation ID must be a numeric string: %v", err)
		return fmt.Errorf("conversation ID must be a numeric string")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	query := `
		INSERT INTO conversations (id, title, mode, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			title = excluded.title,
			mode = excluded.mode,
			updated_at = CURRENT_TIMESTAMP
	`

	_, err := m.db.Exec(query, chatID, title, mode)
	if err != nil {
		return fmt.Errorf("failed to save conversation: %w", err)
	}

	return nil
}

// GetConversation 获取对话信息
func (m *SQLMemory) GetConversation(chatID string) (map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `SELECT id, title, created_at, updated_at, mode FROM conversations WHERE id = ?`

	row := m.db.QueryRow(query, chatID)

	var id, title, mode string
	var createdAt, updatedAt time.Time

	if err := row.Scan(&id, &title, &createdAt, &updatedAt, &mode); err != nil {
		if err == sql.ErrNoRows {
			logging.Warn("Co313nversation with ID %s not found", chatID)
			return nil, fmt.Errorf("conversation not found")
		}
		return nil, fmt.Errorf("failed to get conversation: %w", err)
	}

	return map[string]interface{}{
		"id":         id,
		"title":      title,
		"created_at": createdAt,
		"updated_at": updatedAt,
		"mode":       mode,
	}, nil
}

// ListConversations 列出所有对话
func (m *SQLMemory) ListConversations() ([]map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `SELECT id, title, created_at, updated_at, mode FROM conversations ORDER BY updated_at DESC`

	rows, err := m.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list conversations: %w", err)
	}
	defer rows.Close()

	var conversations []map[string]interface{}

	for rows.Next() {
		var id, title, mode string
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &title, &createdAt, &updatedAt, &mode); err != nil {
			return nil, fmt.Errorf("failed to scan conversation: %w", err)
		}

		conversations = append(conversations, map[string]interface{}{
			"id":         id,
			"title":      title,
			"created_at": createdAt,
			"updated_at": updatedAt,
			"mode":       mode,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating conversations: %w", err)
	}

	return conversations, nil
}

// DeleteConversation 删除对话及其所有消息
func (m *SQLMemory) DeleteConversation(chatID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// SQLite 的外键约束会自动删除相关消息
	_, err := m.db.Exec("DELETE FROM conversations WHERE id = ?", chatID)
	if err != nil {
		return fmt.Errorf("failed to delete conversation: %w", err)
	}

	return nil
}

// SaveMessage 保存消息
func (m *SQLMemory) SaveMessage(chatID, messageID, role, content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	query := `
		INSERT INTO messages (chatID,messageID, role, content, timestamp)
		VALUES (?, ?,?, ?, CURRENT_TIMESTAMP)
	`

	_, err := m.db.Exec(query, chatID, messageID, role, content)
	if err != nil {
		return fmt.Errorf("failed to save message: %w", err)
	}

	return nil
}

func (m *SQLMemory) GetReasoningMessage(chatID string) ([]map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `SELECT chatID, messageID, role, content, timestamp FROM messages WHERE chatID = ? AND role = 'reasoning' ORDER BY timestamp DESC`

	rows, err := m.db.Query(query, chatID)
	if err != nil {
		return nil, fmt.Errorf("failed to get reasoning message: %w", err)
	}
	defer rows.Close()

	var messages []map[string]interface{}

	for rows.Next() {
		var chatID string
		var messageID string
		var role, content string
		var timestamp time.Time

		if err := rows.Scan(&chatID, &messageID, &role, &content, &timestamp); err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		messages = append(messages, map[string]interface{}{
			"chatID":    chatID,
			"messageID": messageID,
			"role":      role,
			"content":   content,
			"timestamp": timestamp,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating reasoning messages: %w", err)
	}

	return messages, nil
}

// GetMessages 获取对话的所有消息
func (m *SQLMemory) GetMessages(chatID string) ([]map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `SELECT chatID, messageID, role, content, timestamp FROM messages WHERE chatID = ? AND role != 'reasoning' ORDER BY timestamp ASC`
	rows, err := m.db.Query(query, chatID)
	if err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}

	defer rows.Close()

	var messages []map[string]interface{}

	for rows.Next() {
		var chatID int
		var messageID string
		var role, content string
		var timestamp time.Time

		if err := rows.Scan(&chatID, &messageID, &role, &content, &timestamp); err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}

		messages = append(messages, map[string]interface{}{
			"chatID":    chatID,
			"messageID": messageID,
			"role":      role,
			"content":   content,
			"timestamp": timestamp,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating messages: %w", err)
	}

	return messages, nil
}

func (m *SQLMemory) GetMessagesByMessageID(messageID string) (map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `SELECT chatID, messageID, role, content, timestamp FROM messages WHERE messageID = ?`

	row := m.db.QueryRow(query, messageID)

	var chatID string
	var msgID string
	var role, content string
	var timestamp time.Time

	if err := row.Scan(&chatID, &msgID, &role, &content, &timestamp); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("message not found")
		}
		return nil, fmt.Errorf("failed to scan message: %w", err)
	}

	return map[string]interface{}{
		"chatID":    chatID,
		"messageID": msgID,
		"role":      role,
		"content":   content,
		"timestamp": timestamp,
	}, nil
}

// GetLastMessage 获取对话的最后一条消息
func (m *SQLMemory) GetLastMessage(chatid string) (map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `
		SELECT id, role, content, timestamp 
		FROM messages 
		WHERE chatID = ? 
		ORDER BY timestamp DESC 
		LIMIT 1
	`

	row := m.db.QueryRow(query, chatid)

	var id int
	var chatID, messageID, role, content string
	var timestamp time.Time

	if err := row.Scan(&id, &chatID, &messageID, &role, &content, &timestamp); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no messages found")
		}
		return nil, fmt.Errorf("failed to get last message: %w", err)
	}

	return map[string]interface{}{
		"id":        id,
		"chatID":    chatID,
		"messageID": messageID,
		"role":      role,
		"content":   content,
		"timestamp": timestamp,
	}, nil
}

func (m *SQLMemory) DelateMessage(chatID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	query := `DELETE FROM messages WHERE chatID = ?`

	_, err := m.db.Exec(query, chatID)
	if err != nil {
		return fmt.Errorf("failed to delete message: %w", err)
	}

	return nil

}

// UpdateConversationMode 更新对话模式
func (m *SQLMemory) UpdateConversationMode(chatID, mode string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	query := `
		UPDATE conversations 
		SET mode = ?, updated_at = CURRENT_TIMESTAMP 
		WHERE id = ?
	`

	_, err := m.db.Exec(query, mode, chatID)
	if err != nil {
		return fmt.Errorf("failed to update conversation mode: %w", err)
	}

	return nil
}

// Close 关闭数据库连接
func (m *SQLMemory) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.db != nil {
		return m.db.Close()
	}

	return nil
}

// Backup 备份数据库
func (m *SQLMemory) Backup(backupPath string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 确保备份目录存在
	if err := os.MkdirAll(filepath.Dir(backupPath), 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	// 获取当前数据库路径
	currentDB, err := os.ReadFile(m.dbPath)
	if err != nil {
		return fmt.Errorf("failed to read database file: %w", err)
	}

	// 写入备份文件
	if err := os.WriteFile(backupPath, currentDB, 0644); err != nil {
		return fmt.Errorf("failed to write backup file: %w", err)
	}

	return nil
}

// Restore 从备份恢复数据库
func (m *SQLMemory) Restore(backupPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查备份文件是否存在
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("backup file does not exist: %w", err)
	}

	// 关闭当前数据库连接
	if m.db != nil {
		if err := m.db.Close(); err != nil {
			return fmt.Errorf("failed to close database: %w", err)
		}
	}

	// 读取备份文件
	backupData, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("failed to read backup file: %w", err)
	}

	// 写入当前数据库路径
	if err := os.WriteFile(m.dbPath, backupData, 0644); err != nil {
		return fmt.Errorf("failed to restore database: %w", err)
	}

	// 重新打开数据库连接
	db, err := sql.Open("sqlite", m.dbPath)
	if err != nil {
		return fmt.Errorf("failed to reopen database: %w", err)
	}

	m.db = db
	return nil
}

// Export 导出对话数据为 JSON 格式
func (m *SQLMemory) Export(chatID string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 获取对话信息
	conversation, err := m.GetConversation(chatID)
	if err != nil {
		return "", fmt.Errorf("failed to get conversation: %w", err)
	}

	// 获取对话消息
	messages, err := m.GetMessages(chatID)
	if err != nil {
		return "", fmt.Errorf("failed to get messages: %w", err)
	}

	// 构建导出数据
	exportData := map[string]interface{}{
		"conversation": conversation,
		"messages":     messages,
	}

	// 转换为 JSON
	jsonData, err := json.MarshalIndent(exportData, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal export data: %w", err)
	}

	return string(jsonData), nil
}

// Import 从 JSON 格式导入对话数据
func (m *SQLMemory) Import(jsonData string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 解析 JSON 数据
	var importData struct {
		Conversation map[string]interface{}   `json:"conversation"`
		Messages     []map[string]interface{} `json:"messages"`
	}

	if err := json.Unmarshal([]byte(jsonData), &importData); err != nil {
		return fmt.Errorf("failed to unmarshal import data: %w", err)
	}

	// 提取对话信息
	chatID, ok := importData.Conversation["id"].(string)
	if !ok {
		return fmt.Errorf("invalid conversation ID")
	}

	title, ok := importData.Conversation["title"].(string)
	if !ok {
		return fmt.Errorf("invalid conversation title")
	}

	mode, ok := importData.Conversation["mode"].(string)
	if !ok {
		mode = "chat"
	}

	// 保存对话
	if err := m.SaveConversation(chatID, title, mode); err != nil {
		return fmt.Errorf("failed to save conversation: %w", err)
	}

	// 保存消息
	for _, msg := range importData.Messages {
		role, ok := msg["role"].(string)
		if !ok {
			return fmt.Errorf("invalid message role")
		}

		content, ok := msg["content"].(string)
		if !ok {
			return fmt.Errorf("invalid message content")
		}
		MessageID, ok := msg["id"].(string)
		if !ok {
			return fmt.Errorf("invalid message id")
		}

		if err := m.SaveMessage(chatID, MessageID, role, content); err != nil {
			return fmt.Errorf("failed to save message: %w", err)
		}
	}

	return nil
}

// GetStats 获取数据库统计信息
func (m *SQLMemory) GetStats() (map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]interface{})

	// 获取对话数量
	var conversationCount int
	if err := m.db.QueryRow("SELECT COUNT(*) FROM conversations").Scan(&conversationCount); err != nil {
		return nil, fmt.Errorf("failed to get conversation count: %w", err)
	}
	stats["conversation_count"] = conversationCount

	// 获取消息数量
	var messageCount int
	if err := m.db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&messageCount); err != nil {
		return nil, fmt.Errorf("failed to get message count: %w", err)
	}
	stats["message_count"] = messageCount

	// 获取数据库大小
	fileInfo, err := os.Stat(m.dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get database size: %w", err)
	}
	stats["db_size_bytes"] = fileInfo.Size()

	return stats, nil
}

// UpdateConversationTitle 更新对话标题
func (m *SQLMemory) UpdateConversationTitle(chatID, title string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	query := `
        UPDATE conversations
        SET title = ?, updated_at = CURRENT_TIMESTAMP
        WHERE id = ?
    `
	_, err := m.db.Exec(query, title, chatID)
	if err != nil {
		return fmt.Errorf("failed to update conversation title: %w", err)
	}
	return nil
}
