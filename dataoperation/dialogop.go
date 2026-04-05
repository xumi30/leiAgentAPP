package dataoperation

import (
	"time"

	"leiAgent/logging"
	"leiAgent/utils"
)

func GetDialogs(chatID string) []map[string]interface{} {
	sql := GetSqlInstance()
	if sql == nil {
		return nil
	}
	messages, err := sql.GetMessages(chatID)
	if err != nil {
		logging.Error("获取消息失败: %v", err)
		return nil
	}

	return messages
}

func GetDialogsByMessageID(messageID string) map[string]interface{} {
	sql := GetSqlInstance()
	if sql == nil {
		return nil
	}

	message, err := sql.GetMessagesByMessageID(messageID)
	logging.Info("GetDialogsByMessageID: %v", message)
	if err != nil {
		logging.Error("GetDialogsByMessageID获取消息列表失败: %v", err)
		return nil
	}
	return message
}

// GetAllMessageContentsForDocHarvest loads all non-reasoning message bodies to discover file paths mentioned in history.
func GetAllMessageContentsForDocHarvest() []string {
	sql := GetSqlInstance()
	if sql == nil {
		return nil
	}
	contents, err := sql.SelectAllMessageContents()
	if err != nil {
		logging.Error("SelectAllMessageContents: %v", err)
		return nil
	}
	return contents
}

func SendMessage(chatID, messageID, message, role string) error {

	sql := GetSqlInstance()

	if sql == nil {
		return nil
	}

	//如果conversation不存在,先创建一个conversation
	conversations, err := sql.GetConversation(chatID)
	if err != nil || conversations == nil {
		logging.Error("SendMessage 没有发现存在的对话id，现在新建对话: %v", err)
		err := sql.SaveConversation(chatID, utils.TruncateRunes(message, utils.ChatTitleMaxRunes), "chat")
		if err != nil {
			logging.Error("创建对话失败: %v", err)
			return err
		}
		logging.Info("创建对话成功")
	}

	err = sql.SaveMessage(chatID, messageID, role, message)
	if err != nil {
		logging.Error("保存消息失败: %v", err)
		return err
	}
	return nil

}

// SendMessageWithCreateTime 写入消息并使用 createTime 作为库内 timestamp（流式首包到达时间，避免仅用收尾写入时刻导致乱序）
func SendMessageWithCreateTime(chatID, messageID, message, role string, createTime time.Time) error {
	sql := GetSqlInstance()
	if sql == nil {
		return nil
	}

	conversations, err := sql.GetConversation(chatID)
	if err != nil || conversations == nil {
		logging.Error("SendMessageWithCreateTime 没有发现存在的对话id，现在新建对话: %v", err)
		err := sql.SaveConversation(chatID, utils.TruncateRunes(message, utils.ChatTitleMaxRunes), "chat")
		if err != nil {
			logging.Error("创建对话失败: %v", err)
			return err
		}
	}

	err = sql.SaveMessageWithTimestamp(chatID, messageID, role, message, createTime)
	if err != nil {
		logging.Error("保存消息失败: %v", err)
		return err
	}
	return nil
}
