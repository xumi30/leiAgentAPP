package dataoperation

import (
	"leiAgent/logging"
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

func SendMessage(chatID, messageID, message, role string) error {

	sql := GetSqlInstance()

	if sql == nil {
		return nil
	}

	//如果conversation不存在,先创建一个conversation
	conversations, err := sql.GetConversation(chatID)
	if err != nil || conversations == nil {
		logging.Error("SendMessage 没有发现存在的对话id，现在新建对话: %v", err)
		err := sql.SaveConversation(chatID, message, "chat")
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
