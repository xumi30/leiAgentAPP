package dispatcher

import (
	"context"
	"strings"

	"leiAgent/internal/profile"
	"leiAgent/logging"
	"leiAgent/utils"
)

func mergeExtraSystemMessages(ctx context.Context, extras ...string) context.Context {
	merged := make([]string, 0, len(extras)+2)
	if existing, ok := ctx.Value(utils.ExtraSystemMessagesString).([]string); ok && len(existing) > 0 {
		merged = append(merged, existing...)
	}
	for _, msg := range extras {
		msg = strings.TrimSpace(msg)
		if msg == "" {
			continue
		}
		merged = append(merged, msg)
	}
	if len(merged) == 0 {
		return ctx
	}
	return context.WithValue(ctx, utils.ExtraSystemMessagesString, merged)
}

func (d *Dispatcher) attachProfileContext(ctx context.Context) context.Context {
	chatID, _ := ctx.Value(utils.ChatIDString).(string)
	if strings.TrimSpace(chatID) == "" {
		return ctx
	}
	directives := profile.BuildSystemDirectives(ctx, chatID)
	if len(directives) == 0 {
		return ctx
	}
	logging.Info("已为 chatID=%s 注入 user profile directives: %d", chatID, len(directives))
	return mergeExtraSystemMessages(ctx, directives...)
}
