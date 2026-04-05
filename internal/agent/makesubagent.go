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
	subChatID, err := sql.GenerateSubChatIDWithChatId(chatID)
	if err != nil {
		return nil, err
	}

	// 必须继承父 ctx：保留取消、deadline、IntentKey 等；只把 ChatID 换成子会话。
	// 禁止在这里 defer cancel：MakeSubAgent 一 return 就会执行 defer，子 Agent 里的 ctx 立刻 Done，
	// HandleChat / Run 会马上以 context canceled 结束。
	subctx := context.WithValue(ctx, utils.ChatIDString, subChatID)

	agent, err := NewAgent(WithSystemPrompt(systemprompt), WithCtx(subctx))
	if err != nil {
		return nil, err
	}

	return agent, nil

}
