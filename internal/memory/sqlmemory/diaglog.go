package sqlmemory

import (
	"database/sql"
	"fmt"
	"leiAgent/logging"
	"leiAgent/utils"
	"time"
)

const dialogtable = `CREATE TABLE IF NOT EXISTS messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    messageID TEXT NOT NULL UNIQUE,
    chatID TEXT NOT NULL,
    role TEXT NOT NULL CHECK(role IN ('user', 'assistant', 'system', 'reasoning')),
    content TEXT NOT NULL,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (chatID) REFERENCES conversations(id) ON DELETE CASCADE
)`

const dialogindexchatID = `CREATE INDEX IF NOT EXISTS idx_messages_chatID ON messages(chatID)`
const dialogindexchatIDRole = `CREATE INDEX IF NOT EXISTS idx_messages_chatID_role ON messages(chatID, role)`
const dialogindextimestamp = `CREATE INDEX IF NOT EXISTS idx_messages_timestamp ON messages(timestamp DESC)`

const subchattable = `CREATE TABLE IF NOT EXISTS chat_subchat (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    chatID TEXT NOT NULL,
    subChatID TEXT NOT NULL UNIQUE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (chatID) REFERENCES conversations(id) ON DELETE CASCADE
)`

const chatIDindex = `CREATE INDEX IF NOT EXISTS idx_chat_subchat_chatID ON chat_subchat(chatID)`
const subChatIDindex = `CREATE INDEX IF NOT EXISTS idx_chat_subchat_subChatID ON chat_subchat(subChatID)`

func (m *SQLMemory) InsertChatSubChat(chatID, subChatID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	query := `INSERT INTO chat_subchat (chatID, subChatID) VALUES (?, ?)`
	_, err := m.db.Exec(query, chatID, subChatID)
	if err != nil {
		return fmt.Errorf("failed to insert chat-subchat relation: %w", err)
	}

	return nil
}

func (m *SQLMemory) GenerateSubChatIDWithChatId(ChatID string) (string, error) {
	subchatId := fmt.Sprintf("sub-%s", utils.GenerateChatID())
	if err := m.InsertChatSubChat(ChatID, subchatId); err != nil {
		logging.Error("插入子对话ID失败: %v", err)
		return "", err
	}
	logging.Info("生成子对话ID: %s, 归属对话ID: %s", subchatId, ChatID)
	return subchatId, nil

}

// GeneratePlanRunIDWithChatID 为一次「计划执行实例」生成隔离用 ID（用于 memory/system prompt 等临时上下文）。
// 注意：该 ID 不会创建新的 conversation，只会记录归属关系，便于调试/追踪。
func (m *SQLMemory) GeneratePlanRunIDWithChatID(chatID string) (string, error) {
	planRunID := fmt.Sprintf("planrun-%s", utils.GenerateChatID())
	if err := m.InsertChatSubChat(chatID, planRunID); err != nil {
		logging.Error("插入 planRunID 失败: %v", err)
		return "", err
	}
	logging.Info("生成 planRunID: %s, 归属对话ID: %s", planRunID, chatID)
	return planRunID, nil
}

// GetChatSubChat 获取子对话的归属关系
func (m *SQLMemory) GetChatSubChat(subChatID string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `SELECT chatID FROM chat_subchat WHERE subChatID = ?`
	var chatID string
	err := m.db.QueryRow(query, subChatID).Scan(&chatID)
	if err != nil {
		return "", fmt.Errorf("failed to get chat-subchat relation: %w", err)
	}
	return chatID, nil
}

// SaveConversation 保存或更新对话

// SaveMessage 保存消息（写入时刻作为 timestamp）
func (m *SQLMemory) SaveMessage(chatID, messageID, role, content string) error {
	return m.SaveMessageWithTimestamp(chatID, messageID, role, content, time.Now())
}

// SaveMessageWithTimestamp 保存消息并指定 timestamp（用于流式首包到达时间，保证会话内排序正确）
func (m *SQLMemory) SaveMessageWithTimestamp(chatID, messageID, role, content string, ts time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ts.IsZero() {
		ts = time.Now()
	}

	query := `
		INSERT INTO messages (chatID, messageID, role, content, timestamp)
		VALUES (?, ?, ?, ?, ?)
	`

	_, err := m.db.Exec(query, chatID, messageID, role, content, ts.UTC())
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

// GetMessagesByChatIDAndRole 按 chatID 与 role 获取消息列表（时间升序，字段与 GetMessages 一致）
func (m *SQLMemory) GetMessagesByChatIDAndRole(chatID, role string) ([]map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `SELECT chatID, messageID, role, content, timestamp FROM messages WHERE chatID = ? AND role = ? ORDER BY timestamp ASC`

	rows, err := m.db.Query(query, chatID, role)
	if err != nil {
		return nil, fmt.Errorf("failed to get messages by role: %w", err)
	}
	defer rows.Close()

	var messages []map[string]interface{}

	for rows.Next() {
		var cid string
		var messageID string
		var r, content string
		var timestamp time.Time

		if err := rows.Scan(&cid, &messageID, &r, &content, &timestamp); err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		messages = append(messages, map[string]interface{}{
			"chatID":    cid,
			"messageID": messageID,
			"role":      r,
			"content":   content,
			"timestamp": timestamp,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating messages by role: %w", err)
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

// SelectAllMessageContents returns every stored message body (for doc path harvest). Excludes reasoning role.
func (m *SQLMemory) SelectAllMessageContents() ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rows, err := m.db.Query(`SELECT content FROM messages WHERE role != 'reasoning' ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("failed to query message contents: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			return nil, fmt.Errorf("failed to scan message content: %w", err)
		}
		out = append(out, content)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating message contents: %w", err)
	}
	return out, nil
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
