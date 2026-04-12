package memoryagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"leiAgent/internal/memory"
	"leiAgent/internal/proxy"
	"leiAgent/utils"
)

const compressSystemPrompt = `你是记忆压缩助手。用户会提供一段完整对话历史，每条消息都标注了消息类型（user / assistant / system / tool）。
请从中提取对「用户与助手」后续对话仍有价值的关键信息，例如：用户偏好与目标、已确认的事实、约定与约束、未完成事项、重要结论等。
输出要求：
- 使用简洁的条目或短段落，不要复述全文，不要编造对话中不存在的内容。
- 若几乎没有可保留的信息，只输出一个字：无`

// Compress 根据 chatID 读取本地记忆中的全部历史，经模型压缩后写回 system 压缩记忆并持久化到 localmemory/{chatID}.yaml。
// 模型请求通过 proxy.Communicate 发送，不修改本次请求前用于构造 prompt 的内存内容（使用 MemoryMessagesOverride）。
func Compress(ctx context.Context, chatID string) (string, error) {
	cid := strings.TrimSpace(chatID)
	if cid == "" {
		return "", fmt.Errorf("chatID 为空")
	}
	raw := memory.GetLocalMemory().GetMessages(cid)
	if len(raw) == 0 {
		return "", fmt.Errorf("无历史消息可压缩")
	}
	historyText := formatHistoryWithRoles(raw)
	userPayload := "以下为待压缩的对话历史（含消息类型标注）：\n\n" + historyText

	override := []*memory.Message{
		{Role: memory.MessageRoleSystem, Content: compressSystemPrompt},
		{Role: memory.MessageRoleUser, Content: userPayload},
	}

	p, err := proxy.NewProxy(nil)
	if err != nil {
		return "", err
	}

	reqCtx := context.WithValue(ctx, utils.ChatIDString, cid)
	reqCtx = context.WithValue(reqCtx, utils.IsStreamString, false)
	reqCtx = context.WithValue(reqCtx, utils.SkipDialogToUIString, true)
	reqCtx = context.WithValue(reqCtx, utils.MemoryMessagesOverrideString, override)

	tc, err := p.Communicate(reqCtx)
	if err != nil {
		return "", err
	}
	if tc == nil {
		return "", fmt.Errorf("模型返回为空")
	}
	summary := strings.TrimSpace(tc.Content)
	summary = strings.TrimSuffix(summary, "\n")
	if summary == "" {
		return "", fmt.Errorf("模型未返回有效压缩内容")
	}

	memory.GetLocalMemory().Clear(cid)
	_ = memory.SetSystemPrompt(cid, "【压缩记忆】\n"+summary)

	if err := memory.PersistLocalMemoryToYAMLFile(cid); err != nil {
		return summary, fmt.Errorf("已生成压缩文本但写入 YAML 失败: %w", err)
	}
	return summary, nil
}

func formatHistoryWithRoles(msgs []*memory.Message) string {
	var b strings.Builder
	idx := 0
	for _, m := range msgs {
		if m == nil {
			continue
		}
		idx++
		fmt.Fprintf(&b, "── 第 %d 条 · 类型=%s ──\n", idx, string(m.Role))
		if c := strings.TrimSpace(m.Content); c != "" {
			b.WriteString(c)
			b.WriteByte('\n')
		}
		if len(m.ToolCalls) > 0 {
			if j, err := json.Marshal(m.ToolCalls); err == nil {
				fmt.Fprintf(&b, "[tool_calls] %s\n", string(j))
			}
		}
		if m.ToolCallID != "" {
			fmt.Fprintf(&b, "[tool_call_id] %s\n", m.ToolCallID)
		}
		b.WriteByte('\n')
	}
	return b.String()
}
