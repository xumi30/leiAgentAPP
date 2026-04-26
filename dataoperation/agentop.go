package dataoperation

import "fmt"

func SyncPresetAgents() error {
	sql := GetSqlInstance()
	if sql == nil {
		return fmt.Errorf("无法获取数据库实例")
	}
	return sql.SyncPresetAgents()
}

func ListAgents() ([]map[string]interface{}, error) {
	sql := GetSqlInstance()
	if sql == nil {
		return nil, fmt.Errorf("无法获取数据库实例")
	}
	return sql.ListAgents()
}

func ListConversationAgents(chatID string) ([]map[string]interface{}, error) {
	sql := GetSqlInstance()
	if sql == nil {
		return nil, fmt.Errorf("无法获取数据库实例")
	}
	conv, err := sql.GetConversation(chatID)
	if err != nil {
		return nil, err
	}
	ids, _ := conv["agents"].([]string)
	return sql.ListAgentsByIDs(ids)
}

func CreateAgent(agentID, agentName, avatarImage, description string) (map[string]interface{}, error) {
	sql := GetSqlInstance()
	if sql == nil {
		return nil, fmt.Errorf("无法获取数据库实例")
	}
	if err := sql.SaveAgentWithName(agentID, agentName, avatarImage, description); err != nil {
		return nil, err
	}
	return sql.GetAgent(agentID)
}

func DeleteCustomAgent(agentID string) error {
	sql := GetSqlInstance()
	if sql == nil {
		return fmt.Errorf("无法获取数据库实例")
	}
	return sql.DeleteCustomAgent(agentID)
}

func GetAgent(agentID string) (map[string]interface{}, error) {
	sql := GetSqlInstance()
	if sql == nil {
		return nil, fmt.Errorf("无法获取数据库实例")
	}
	return sql.GetAgent(agentID)
}
