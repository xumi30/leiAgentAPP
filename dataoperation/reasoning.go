package dataoperation

import "fmt"

func GetReasonings(chatid string) ([]map[string]interface{}, error) {
	sql := GetSqlInstance()

	if sql == nil {
		return nil, fmt.Errorf("sql is nil")
	}

	rs, er := sql.GetReasoningMessage(chatid)
	return rs, er

}
