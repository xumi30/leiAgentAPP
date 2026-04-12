package memoryagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"leiAgent/internal/memory"
	"leiAgent/internal/provider/openaistyle"
	"leiAgent/internal/proxy"
	"leiAgent/utils"
)

const (
	compressSummaryPrefix = "【压缩记忆】\n"
	keepRecentMessageTail = 8
	minMessagesToCompress = 6
)

const compressSystemPrompt = `你是记忆压缩助手。用户会提供一段较早的对话历史，以及一段最近的保留消息。
你的任务是把“较早历史”压缩成一段供后续对话继续使用的长期记忆。

压缩时请优先保留：
- 用户偏好、目标、约束、习惯
- 已确认的重要事实、结论、约定
- 未完成事项、后续承诺
- 对后续回答仍有帮助的上下文

不要保留：
- 纯闲聊寒暄
- 一次性中间步骤
- 冗长工具日志原文
- 已经失效的系统提示词

输出要求：
- 只输出压缩后的记忆正文，不要解释，不要加标题
- 使用简洁条目或短段落
- 不要编造对话中不存在的内容
- 如果几乎没有值得保留的信息，只输出一个字：无`

var (
	compressingChats sync.Map
)

type compressionPlan struct {
	existingSummary string
	toCompress      []*memory.Message
	toKeep          []*memory.Message
}

// Compress 根据 chatID 读取会话历史，压缩较早消息并保留最近消息，避免把整段会话直接清空成一条摘要。
func Compress(ctx context.Context, chatID string) (string, error) {
	cid := strings.TrimSpace(chatID)
	if cid == "" {
		return "", fmt.Errorf("chatID 为空")
	}
	if _, loaded := compressingChats.LoadOrStore(cid, struct{}{}); loaded {
		return "", fmt.Errorf("chatID=%s 正在压缩中", cid)
	}
	defer compressingChats.Delete(cid)

	raw := memory.GetLocalMemory().GetMessages(cid)
	plan := buildCompressionPlan(raw)
	if len(plan.toCompress) == 0 {
		if strings.TrimSpace(plan.existingSummary) != "" {
			return strings.TrimSpace(plan.existingSummary), nil
		}
		return "", fmt.Errorf("无可压缩历史消息")
	}

	userPayload := buildCompressionPayload(plan)
	p, err := proxy.NewProxy(nil)
	if err != nil {
		return "", err
	}

	reqCtx := context.WithValue(ctx, utils.ChatIDString, cid)
	reqCtx = context.WithValue(reqCtx, utils.IsStreamString, false)
	reqCtx = context.WithValue(reqCtx, utils.SkipDialogToUIString, true)

	tc, err := p.CommunicateWithMessages(reqCtx, []openaistyle.ChatMessage{
		{Role: openaistyle.RoleSystem, Content: compressSystemPrompt},
		{Role: openaistyle.RoleUser, Content: userPayload},
	})
	if err != nil {
		return "", err
	}
	if tc == nil {
		return "", fmt.Errorf("模型返回为空")
	}

	newSummary := normalizeSummary(tc.Content)
	if newSummary == "" {
		return "", fmt.Errorf("模型未返回有效压缩内容")
	}

	mergedSummary := mergeSummaries(plan.existingSummary, newSummary)
	restoreCompressedMemory(cid, mergedSummary, plan.toKeep)

	if err := memory.PersistLocalMemoryToYAMLFile(cid); err != nil {
		return mergedSummary, fmt.Errorf("已生成压缩文本但写入 YAML 失败: %w", err)
	}
	return mergedSummary, nil
}

func buildCompressionPlan(msgs []*memory.Message) compressionPlan {
	filtered := make([]*memory.Message, 0, len(msgs))
	var existingSummary string
	for _, msg := range msgs {
		if msg == nil {
			continue
		}
		if msg.Role == memory.MessageRoleSystem {
			if strings.HasPrefix(strings.TrimSpace(msg.Content), strings.TrimSpace(compressSummaryPrefix)) {
				existingSummary = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(msg.Content), strings.TrimSpace(compressSummaryPrefix)))
			}
			continue
		}
		filtered = append(filtered, cloneMessage(msg))
	}

	if len(filtered) <= keepRecentMessageTail || len(filtered) < minMessagesToCompress {
		return compressionPlan{
			existingSummary: existingSummary,
			toKeep:          filtered,
		}
	}

	split := len(filtered) - keepRecentMessageTail
	if split < 0 {
		split = 0
	}

	return compressionPlan{
		existingSummary: existingSummary,
		toCompress:      filtered[:split],
		toKeep:          filtered[split:],
	}
}

func buildCompressionPayload(plan compressionPlan) string {
	var b strings.Builder
	if s := strings.TrimSpace(plan.existingSummary); s != "" {
		b.WriteString("这是已有的长期记忆摘要，请在此基础上增量整合，而不是重复改写无关内容：\n")
		b.WriteString(s)
		b.WriteString("\n\n")
	}

	b.WriteString("以下是需要压缩的较早历史消息：\n\n")
	b.WriteString(formatHistoryWithRoles(plan.toCompress))

	if len(plan.toKeep) > 0 {
		b.WriteString("\n以下是最近保留、不需要压缩但需要你参考的消息：\n\n")
		b.WriteString(formatHistoryWithRoles(plan.toKeep))
	}
	return b.String()
}

func restoreCompressedMemory(chatID, summary string, recent []*memory.Message) {
	lm := memory.GetLocalMemory()
	lm.Clear(chatID)
	_ = memory.SetSystemPrompt(chatID, compressSummaryPrefix+summary)
	for _, msg := range recent {
		if msg == nil {
			continue
		}
		_ = lm.AddMessage(chatID, cloneMessage(msg))
	}
}

func mergeSummaries(existing, current string) string {
	cur := normalizeSummary(current)
	if cur == "" {
		return normalizeSummary(existing)
	}
	old := normalizeSummary(existing)
	if old == "" || cur == "无" {
		if cur == "无" && old != "" {
			return old
		}
		return cur
	}
	if strings.Contains(cur, old) {
		return cur
	}
	return cur
}

func normalizeSummary(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, compressSummaryPrefix)
	s = strings.TrimSpace(s)
	return strings.TrimSuffix(s, "\n")
}

func cloneMessage(m *memory.Message) *memory.Message {
	if m == nil {
		return nil
	}
	cp := &memory.Message{
		Role:       m.Role,
		Content:    m.Content,
		ToolCallID: m.ToolCallID,
	}
	if len(m.ToolCalls) > 0 {
		cp.ToolCalls = append([]memory.ToolCall(nil), m.ToolCalls...)
	}
	return cp
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
