package planner

import (
	"context"
	"fmt"
	globalchannel "leiAgent/internal"
	"leiAgent/internal/memory"
	"leiAgent/logging"
)

func (planner *Planning) DoTask(ctx context.Context) (string, error) {
	chatId, ok := ctx.Value("chatId").(string)
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
			dialogOutputChan <- fmt.Sprintf("执行规划失败倒数第%d次: %v", planner.RetryCount, err)
			memory.AddUserMessage(chatId, fmt.Sprintf("执行规划失败倒数第%d次: %v", planner.RetryCount, err))
		}

	}

	memory.AddUserMessage(chatId, "全部规划尝试执行完成，以下是执行结果："+pstr)
	dialogOutputChan <- "规划执行完成，以下是执行结果：" + pstr

	return pstr, nil

}
