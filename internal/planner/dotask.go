package planner

import (
	"context"
	"fmt"
	"strings"

	"leiAgent/internal/globalchannel"
	"leiAgent/internal/memory"
	"leiAgent/internal/proxy"
	"leiAgent/logging"
	"leiAgent/utils"
)

func (planner *Planning) DoTask(ctx context.Context) (string, error) {
	logging.Info("开始执行规划...")
	chatId, ok := ctx.Value(utils.ChatIDString).(string)
	if !ok {
		logging.Error("chatId is not found in context")
		return "", fmt.Errorf("chatId is not found in context")
	}
	dialogOutputChan := globalchannel.GetGlobalDialogOutChannel(chatId)

	pstr, err := planner.DoExe(ctx)

	if err != nil {
		logging.Error("第一次执行规划失败: %v", err)
		memory.AddUserMessage(chatId, "第一次执行规划失败，返回的错误是："+err.Error())
	}

	for planner.Status == "failed" && planner.RetryCount > 0 {
		fmt.Printf("执行规划失败，正在进行倒数第%d次重试...\n", planner.RetryCount)
		planner.RetryCount--
		retryResult, err := planner.VerifyResult(ctx, pstr)
		if err != nil {
			logging.Error("Failed to retry verify result: %v", err)
			return "", err
		}

		// 错误信息
		memory.AddUserMessage(chatId, "执行规划失败倒数第"+fmt.Sprint(planner.RetryCount)+"次，以下是重试后的结果："+retryResult)
		pstr, err = planner.DoExe(ctx)

		if err != nil {
			logging.Error("执行规划失败倒数第%d次: %v", planner.RetryCount, err)
			dialogOutputChan <- &globalchannel.Message{Content: fmt.Sprintf("执行规划失败倒数第%d次: %v", planner.RetryCount, err), Role: utils.MessageRoleAssistant, IsFinished: false}
			memory.AddUserMessage(chatId, fmt.Sprintf("执行规划失败倒数第%d次: %v", planner.RetryCount, err))
		}

	}

	memory.AddUserMessage(chatId, "全部规划尝试执行完成，以下是执行结果："+pstr)

	if err := summarizePlanExecution(ctx, chatId); err != nil {
		logging.Warn("执行总结未生成: %v", err)
		dialogOutputChan <- &globalchannel.Message{Content: "\n规划执行已完成。总结生成失败，执行明细已写入对话记录（JSON）。\n", Role: utils.MessageRoleAssistant, IsFinished: false}
	}

	return pstr, nil

}

// summarizePlanExecution 根据记忆中上一条执行结果 JSON，调用模型输出面向用户的总结（流式写入对话区）。
func summarizePlanExecution(ctx context.Context, chatId string) error {
	dialogOutputChan := globalchannel.GetGlobalDialogOutChannel(chatId)
	dialogOutputChan <- &globalchannel.Message{Content: "\n正在生成执行总结…\n", Role: utils.MessageRoleAssistant, IsFinished: false}
	dialogOutputChan <- &globalchannel.Message{Content: utils.FinishString, Role: utils.MessageRoleAssistant, IsFinished: true}

	memory.GetLocalMemory().SetSystemPrompt(chatId, PlanSummarySystemPrompt)
	memory.AddUserMessage(chatId, "请根据上一条消息里的计划执行结果（JSON），生成给用户的完整总结（目标、各步结果、总体结论与建议）。遵守系统提示中的格式与语言要求。")

	p, err := proxy.NewProxy(nil)
	if err != nil {
		return fmt.Errorf("创建 LLM 代理失败: %w", err)
	}

	tc, err := p.Communicate(ctx)
	if err != nil {
		return err
	}
	if tc != nil && strings.TrimSpace(tc.Content) != "" {
		memory.AddAssistantContentMessage(chatId, tc.Content)
	}
	return nil
}
