package agent

import (
	"context"
	"errors"
	"leiAgent/internal/memory/sqlmemory"
	"leiAgent/utils"
)

func MakeSubAgent(ctx context.Context, systemprompt string) (*Agent, error) {

	chatID, ok := ctx.Value(utils.ChatIDString).(string)
	if !ok {
		return nil, errors.New("chatID not found in context")
	}
	sql, err := sqlmemory.GetSqlInstance("")
	if err != nil {
		return nil, err
	}
	subChatId, err := sql.GenerateSubChatIDWithChatId(chatID)
	if err != nil {
		return nil, err
	}

	subctx, subcancel := context.WithCancel(context.Background())
	defer subcancel()
	subctx = context.WithValue(subctx, utils.ChatIDString, subChatId)

	agent, err := NewAgent(WithSystemPrompt(systemprompt), WithCtx(subctx))
	if err != nil {
		return nil, err
	}

	return agent, nil

}
