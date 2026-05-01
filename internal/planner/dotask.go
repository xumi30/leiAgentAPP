package planner

import (
	"context"
	"fmt"
	"strings"

	"leiAgent/internal/globalchannel"
	"leiAgent/internal/memory"
	"leiAgent/internal/provider/openaistyle"
	"leiAgent/internal/proxy"
	"leiAgent/logging"
	"leiAgent/utils"
)

const PlanSummarySystemPrompt = `You summarize a completed multi-step plan execution for the end user.

Input context contains JSON with "goal", "steps" (id, tool, status, result, error fields), and plan status.

Rules:
- Write in the same language as the user's goal when you can infer it.
- Use Markdown: headings and bullet lists are encouraged.
- Cover: what they asked for; what each step did and whether it succeeded; key outcomes (summarize large results, do not paste huge blobs).
- End with overall outcome (full success / partial / failed) and short next-step suggestions if helpful.
- Do not repeat the full raw JSON.`

func (planner *Planning) DoTask(ctx context.Context) (string, error) {
	logging.Info("开始执行规划...")
	chatId, ok := ctx.Value(utils.ChatIDString).(string)
	if !ok {
		logging.Error("chatId is not found in context")
		return "", fmt.Errorf("chatId is not found in context")
	}

	pstr, err := planner.DoExe(ctx)

	if err != nil {
		logging.Error("第一次执行规划失败: %v", err)
		globalchannel.SendAssistantMessageOnce(ctx, fmt.Sprintf("第一次执行规划失败: %v", err))
	}

	for planner.Status != utils.TaskCompleted && planner.RetryCount > 0 {
		fmt.Printf("执行规划失败，正在进行倒数第%d次重试...\n", planner.RetryCount)
		globalchannel.SendAssistantMessageOnce(ctx, fmt.Sprintf("执行规划失败，正在进行倒数第%d次重试...\n", planner.RetryCount))
		planner.RetryCount--
		retryResult, err := planner.VerifyResult(ctx, pstr)

		if err != nil {
			logging.Error("Failed to retry verify result: %v", err)
			globalchannel.SendAssistantMessageOnce(ctx, fmt.Sprintf("Failed to retry verify result: %v", err))
			return "", err
		}

		// 错误信息
		globalchannel.SendAssistantMessageOnce(ctx, fmt.Sprintf("计划执行失败倒数第%d次，以下是重试后的结果：%s", planner.RetryCount, retryResult))
		pstr, err = planner.DoExe(ctx)

		if err != nil {
			logging.Error("执行规划失败倒数第%d次: %v", planner.RetryCount, err)

			globalchannel.SendAssistantMessageOnce(ctx, fmt.Sprintf("执行规划失败倒数第%d次: %v", planner.RetryCount, err))
			return "", err
		}

	}

	globalchannel.SendAssistantMessageOnce(ctx, fmt.Sprintf("全部规划尝试执行完成，以下是执行结果：%s", pstr))
	logging.Info("全部规划尝试执行完成，以下是执行结果：%s", pstr)

	if planner.Status == utils.TaskFailed {
		logging.Error("执行规划失败: %v", planner.Status)
		globalchannel.SendAssistantMessageOnce(ctx, fmt.Sprintf("执行规划失败: %v", planner.Status))
		// return "", fmt.Errorf("执行规划失败: %v", planner.Status)
	}

	if err := summarizePlanExecution(ctx, chatId); err != nil {
		logging.Warn("执行总结未生成: %v", err)
		globalchannel.SendAssistantMessageOnce(ctx, fmt.Sprintf("执行总结未生成: %v", err))
	}
	globalchannel.SendAssistantMessageOnce(ctx, "执行总结已生成。")

	return pstr, nil

}

// summarizePlanExecution 根据记忆中上一条执行结果 JSON，调用模型输出面向用户的总结（流式写入对话区）。
func summarizePlanExecution(ctx context.Context, chatId string) error {

	globalchannel.SendAssistantMessageOnce(ctx, fmt.Sprintf("%s", "正在生成执行总结…\n"))

	p, err := proxy.NewClient(nil)
	if err != nil {
		return fmt.Errorf("创建 LLM 代理失败: %w", err)
	}

	reqCtx := context.WithValue(ctx, utils.ChatIDString, chatId)
	reqCtx = context.WithValue(reqCtx, utils.SkipDialogToUIString, true)
	msgs := []openaistyle.ChatMessage{{
		Role:    openaistyle.RoleSystem,
		Content: PlanSummarySystemPrompt,
	}}
	msgs = append(msgs, historyToChatMessages(memory.GetLocalMemory().GetMessages(chatId))...)
	msgs = append(msgs, openaistyle.ChatMessage{
		Role:    openaistyle.RoleUser,
		Content: "综合以上信息生成面向用户goal的总结。不要强调执行了哪些步骤，而是强调最终结果。不要关注错误步骤，只关注成功获取的信息,然后结合你的相关知识补充扩展,给出兼具理性和感性的回复。",
	})
	tc, err := p.CommunicateWithMessages(reqCtx, msgs)
	if err != nil {
		return err
	}
	if tc != nil && strings.TrimSpace(tc.Content) != "" {
		memory.AddAssistantContentMessage(chatId, tc.Content)
	}
	return nil
}

func historyToChatMessages(msgs []*memory.Message) []openaistyle.ChatMessage {
	out := make([]openaistyle.ChatMessage, 0, len(msgs))
	for _, msg := range msgs {
		if msg == nil {
			continue
		}
		if strings.TrimSpace(msg.Content) == "" && len(msg.ToolCalls) == 0 && msg.ToolCallID == "" {
			continue
		}
		chatMsg := openaistyle.ChatMessage{
			Role:    string(msg.Role),
			Content: msg.Content,
		}
		if msg.ToolCallID != "" {
			chatMsg.ToolCallID = msg.ToolCallID
		}
		if len(msg.ToolCalls) > 0 {
			tc := make([]openaistyle.ToolCall, 0, len(msg.ToolCalls))
			for _, call := range msg.ToolCalls {
				tc = append(tc, openaistyle.ToolCall{
					ID:   call.ID,
					Type: call.Type,
					Function: &openaistyle.Function{
						Name: call.Function.Name,
					},
					Index: call.Index,
				})
			}
			chatMsg.ToolCalls = tc
		}
		out = append(out, chatMsg)
	}
	return out
}
