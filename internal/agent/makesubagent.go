package agent

import (
	"context"
	"errors"
	"leiAgent/dataoperation"
	"leiAgent/utils"
)

func MakeSubAgent(ctx context.Context, systemprompt string) (*Agent, error) {

	chatID, ok := ctx.Value(utils.ChatIDString).(string)
	if !ok {
		return nil, errors.New("chatID not found in context")
	}
	sql := dataoperation.GetSqlInstance()
	if sql == nil {
		return nil, errors.New("database not available")
	}
	planRunID, err := sql.GeneratePlanRunIDWithChatID(chatID)
	if err != nil {
		return nil, err
	}

	// 必须继承父 ctx：保留取消、deadline、IntentKey 等；只把 ChatID 换成子会话。
	// 禁止在这里 defer cancel：MakeSubAgent 一 return 就会执行 defer，子 Agent 里的 ctx 立刻 Done，
	// HandleChat / Run 会马上以 context canceled 结束。
	// memory 使用 planRunID 隔离；UI 输出强制回到父 chatID（已注册的会话）。
	subctx := context.WithValue(ctx, utils.ChatIDString, planRunID)
	subctx = context.WithValue(subctx, utils.DialogOutChatIDString, chatID)

	agent, err := NewAgent(WithSystemPrompt(systemprompt), WithCtx(subctx))
	if err != nil {
		return nil, err
	}

	return agent, nil

}
