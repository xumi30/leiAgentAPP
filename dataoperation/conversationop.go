package dataoperation

import (
	"fmt"
	"leiAgent/internal/memory/sqlmemory"
	"leiAgent/logging"
	"path/filepath"
)

// Dialog represents a conversation structure

func GetSqlInstance() *sqlmemory.SQLMemory {
	dbPath := filepath.Join(".", "data", "memory.db")
	sql, err := sqlmemory.GetSqlInstance(dbPath)
	if err != nil {
		logging.Error("获取数据库实例失败: %v", err)
		return nil
	}
	return sql
}
func ListConversations() []map[string]interface{} {
	if GetSqlInstance() == nil {
		return nil
	}
	conversations, err := GetSqlInstance().ListConversations()

	if err != nil {
		logging.Error("获取对话列表失败: %v", err)
		return nil
	}

	// 处理可能为 nil 的字段
	for _, conversation := range conversations {
		// 确保 mode 字段不为 nil
		if conversation["mode"] == nil {
			conversation["mode"] = "chat"
		}
	}

	// 根据id查询message 如果对话内容为空，则删除
	var validConversations []map[string]interface{}
	for _, conversation := range conversations {
		chatID := conversation["id"].(string) // 使用 "id" 而不是 "chatID"
		messages, err := GetSqlInstance().GetMessages(chatID)
		if err != nil {
			logging.Error("获取对话失败: %v", err)
			continue
		}
		if len(messages) > 0 {
			validConversations = append(validConversations, conversation)
		} else {
			logging.Info("对话内容为空，删除对话: %s", chatID)
			err := DeleteConversation(chatID)
			if err != nil {
				logging.Error("删除对话失败: %v", err)
			}
		}
	}

	return validConversations
}

func GetConversation(chatID string) map[string]interface{} {
	if GetSqlInstance() == nil {
		return nil
	}
	conversation, err := GetSqlInstance().GetConversation(chatID)
	if err != nil {
		logging.Error("获取对话失败: %v", err)
		return nil
	}
	return conversation
}

func AddConversation(chatID string, title string) error {
	if chatID == "" || title == "" {
		return fmt.Errorf("chatID和title不能为空")
	}
	sql := GetSqlInstance()
	if sql == nil {
		return fmt.Errorf("无法获取数据库实例")
	}
	logging.Info("Adding with ID: %s and Title: %s", chatID, title)
	err := sql.SaveConversation(chatID, title, "chat")
	if err != nil {
		logging.Error("添加对话失败: %v", err)
		return err
	}
	return nil
}

func DeleteConversation(chatID string) error {
	sql := GetSqlInstance()
	if sql == nil {
		return fmt.Errorf("无法获取数据库实例")
	}
	err := sql.DeleteConversation(chatID)
	if err != nil {
		logging.Error("删除对话失败: %v", err)
		return err
	}

	//删除对话对应的聊天记录
	err = sql.DeleteMessages(chatID)
	if err != nil {
		logging.Error("删除对话失败: %v", err)
		return err
	}

	return nil
}

func UpdateConversationTitle(chatID string, newTitle string) error {
	sql := GetSqlInstance()
	if sql == nil {
		return fmt.Errorf("无法获取数据库实例")
	}
	err := sql.UpdateConversationTitle(chatID, newTitle)
	if err != nil {
		logging.Error("更新对话标题失败: %v", err)
		return err
	}
	return nil
}

func AddAgentToConversation(chatID, agentID string) error {
	sql := GetSqlInstance()
	if sql == nil {
		return fmt.Errorf("无法获取数据库实例")
	}
	return sql.AddAgentToConversation(chatID, agentID)
}

func ListAgentsInConversation(chatID string) ([]string, error) {
	sql := GetSqlInstance()
	if sql == nil {
		return nil, fmt.Errorf("无法获取数据库实例")
	}
	return sql.ListConversationAgents(chatID)
}
