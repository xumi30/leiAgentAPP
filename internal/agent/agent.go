package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	mcpbridge "leiAgent/internal/MCP"
	"leiAgent/internal/globalchannel"
	"leiAgent/internal/memory"
	"leiAgent/internal/provider/openaistyle"
	"leiAgent/internal/proxy"
	"leiAgent/internal/shellapproval"
	"leiAgent/internal/tools"
	"leiAgent/internal/tools/bashfunction"
	"leiAgent/logging"
	"leiAgent/utils"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Agent struct {
	systemPrompt  string
	proxy         *proxy.Proxy
	taskLoopTimes int
	ctx           context.Context
}

const defaultTaskLoopLimit = 8

type options func(*Agent)

func WithSystemPrompt(description string) options {
	return func(a *Agent) {
		a.systemPrompt = description
	}
}

func WithCtx(ctx context.Context) options {
	return func(a *Agent) {
		a.ctx = ctx
	}
}

func NewAgent(opts ...options) (*Agent, error) {
	p, err := proxy.NewProxy(nil)
	if err != nil {
		return nil, err
	}
	a := &Agent{
		proxy: p,
	}
	for _, opt := range opts {
		opt(a)
	}
	a.taskLoopTimes = defaultTaskLoopLimit

	return a, nil
}

// SetCtx 在会话中断后替换根 context（与 Dispatcher 换 ctx 同步，避免内部仍引用已取消的 ctx）。
func (a *Agent) SetCtx(ctx context.Context) {
	if a == nil {
		return
	}
	a.ctx = ctx
}
func (a *Agent) Run() (string, error) {
	chatId, ok := a.ctx.Value(utils.ChatIDString).(string)
	if !ok || chatId == "" {
		return "", fmt.Errorf("chatId is not found in context")
	}
	inputChan := globalchannel.GetGlobalInputChannel(chatId)

	for {
		select {
		case <-a.ctx.Done():
			return "", a.ctx.Err()
		case msg := <-inputChan:
			if msg == nil || msg.Content == "" {
				continue
			}
			logging.Debug("收到消息: %s", msg.Content)
			rp, err := a.HandleChat(a.ctx, msg.Content)
			if err != nil {
				logging.Error("处理消息失败: %v", err)
			}
			logging.Debug("返回消息: %s", rp)
		}
	}
}

func (a *Agent) HandleChat(ctx context.Context, message string) (string, error) {

	logging.Info("Agent begin to handle chat")

	chatId := ctx.Value(utils.ChatIDString).(string)
	logging.Info("chatId: %s", chatId)

	if a.systemPrompt != "" {
		memory.SetSystemPrompt(chatId, a.systemPrompt)
	}
	memory.AddUserMessage(chatId, message)

	toolAndContent, err := a.proxy.Communicate(ctx)
	logging.Info("代理返回信息: %v", toolAndContent)

	if err != nil {
		return "", fmt.Errorf("通信失败: %w", err)
	}

	if toolAndContent == nil {
		return "", fmt.Errorf("代理返回空内容")
	}

	a.recordMemoryFromResponse(ctx, toolAndContent)

	return toolAndContent.Content, nil
}

func (a *Agent) BeginTask(ctx context.Context, message string) (string, error) {
	a.taskLoopTimes = defaultTaskLoopLimit
	return a.HandleChat(ctx, message)
}

func (a *Agent) handleToolCompleteChat(ctx context.Context, message string) (utils.ToolCompletePayload, error) {
	logging.Info("Agent begin to handle tool-complete chat")

	chatId := ctx.Value(utils.ChatIDString).(string)
	memory.AddUserMessage(chatId, message)

	toolCtx := context.WithValue(ctx, utils.IsStreamString, true)

	toolAndContent, err := a.proxy.Communicate(toolCtx)
	logging.Info("工具完成后代理返回信息: %v", toolAndContent)
	if err != nil {
		return utils.ToolCompletePayload{}, fmt.Errorf("通信失败: %w", err)
	}
	if toolAndContent == nil {
		return utils.ToolCompletePayload{}, fmt.Errorf("代理返回空内容")
	}

	if len(toolAndContent.ToolList) > 0 {
		a.recordMemoryFromResponse(toolCtx, toolAndContent)
		return utils.ToolCompletePayload{}, nil
	}

	payload, ok := utils.ParseToolCompletePayload(toolAndContent.Content)
	if !ok {
		content := strings.TrimSpace(toolAndContent.Content)
		if content != "" {
			memory.AddAssistantContentMessage(chatId, content)
		}
		return utils.ToolCompletePayload{Content: content}, nil
	}

	if strings.TrimSpace(payload.Content) != "" {
		memory.AddAssistantContentMessage(chatId, strings.TrimSpace(payload.Content))
	}

	compactSummary := strings.TrimSpace(payload.SummaryForNextLLM)
	if compactSummary == "" {
		compactSummary = strings.TrimSpace(payload.Content)
	}
	if compactSummary != "" {
		memory.CompactLatestToolRun(chatId, "【工具执行结果压缩摘要，供下一轮使用】\n"+compactSummary)
	}

	return payload, nil
}

// toolCodeRegex 匹配模型以纯文本输出的 <tool_code>...</tool_code> 格式
var toolCodeRegex = regexp.MustCompile(`(?s)<tool_code>\s*(.*?)\s*</tool_code>`)

func (a *Agent) recordMemoryFromResponse(ctx context.Context, toolAndContent *proxy.ToolAndContent) {

	logging.Info("开始记忆返回信息")

	chatId := ctx.Value(utils.ChatIDString).(string)

	if len(toolAndContent.ToolList) > 0 {
		// 原生 tool_calls：正常执行
		names := make([]string, 0, len(toolAndContent.ToolList))
		for _, tc := range toolAndContent.ToolList {
			if tc.Function.Name != "" {
				names = append(names, tc.Function.Name)
			}
		}
		logging.Info("开始执行工具: count=%d names=%s", len(toolAndContent.ToolList), strings.Join(names, ","))
		a.executeTools(ctx, toolAndContent)
	} else if toolAndContent.Content != "" && strings.Contains(toolAndContent.Content, "<tool_code>") {
		// ToolList 为空但 Content 包含 <tool_code>：尝试解析为原生 tool_calls
		parsed := tryParseToolCode(toolAndContent.Content)
		if len(parsed) > 0 {
			logging.Info("检测到 <tool_code> 文本格式，解析出 %d 个工具调用，转为原生 tool_calls 执行", len(parsed))
			a.executeTools(ctx, &proxy.ToolAndContent{ToolList: parsed})
		} else {
			logging.Warn("检测到 <tool_code> 文本但解析失败，清除该段落后存入记忆")
		}
		// 无论解析成功与否，都清除 content 中的 <tool_code> 段，防止污染后续历史
		cleaned := toolCodeRegex.ReplaceAllString(toolAndContent.Content, "")
		cleaned = strings.TrimSpace(cleaned)
		if cleaned != "" {
			memory.AddAssistantContentMessage(chatId, cleaned)
		}
		return
	} else {
		logging.Info("本轮模型未触发工具调用（ToolList 为空）")
	}

	if toolAndContent.Content != "" {
		memory.AddAssistantContentMessage(chatId, toolAndContent.Content)
	}
}

// tryParseToolCode 从包含 <tool_code>...</tool_code> 的文本中解析出原生 tool_calls
func tryParseToolCode(content string) []openaistyle.ChatCompletionToolCall {
	matches := toolCodeRegex.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}

	var tools []openaistyle.ChatCompletionToolCall
	for i, match := range matches {
		body := strings.TrimSpace(match[1])
		var tc struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal([]byte(body), &tc); err != nil {
			logging.Warn("解析第 %d 个 <tool_code> 失败: %v, body: %s", i+1, err, body)
			return nil // 任一失败则整体放弃
		}
		argsStr := string(tc.Arguments)
		if argsStr == "" {
			argsStr = "{}"
		}
		tools = append(tools, openaistyle.ChatCompletionToolCall{
			ID:   fmt.Sprintf("toolcode_%d", i),
			Type: "function",
			Function: &openaistyle.FunctionCall{
				Name:      tc.Name,
				Arguments: argsStr,
			},
			Index: i,
		})
	}
	return tools
}

func truncateForLog(s string, max int) string {
	if max <= 0 {
		return ""
	}
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}

func (a *Agent) executeTools(ctx context.Context, toolAndContent *proxy.ToolAndContent) {

	chatId := ctx.Value(utils.ChatIDString).(string)
	toolCompletePrompt := utils.ToolCompletePromptTemplate
	directCompleteOnly := len(toolAndContent.ToolList) > 0
	toolResults := make([]string, 0, len(toolAndContent.ToolList))

	toolCalls := make([]memory.ToolCall, 0, len(toolAndContent.ToolList))

	for _, tool := range toolAndContent.ToolList {
		jsonStr, err := json.Marshal(tool)
		if err != nil {
			logging.Error("工具信息序列化失败: %v", err)
			continue
		}
		logging.Info("执行工具信息: %s", jsonStr)

		toolCalls = append(toolCalls, memory.ToolCall{
			ID:   tool.ID,
			Type: tool.Type,
			Function: memory.ToolCallFunction{
				Name:      tool.Function.Name,
				Arguments: tool.Function.Arguments,
			},
			Index: tool.Index,
		})
	}
	memory.AddAssistantToolCallsMessage(chatId, toolCalls)

	for _, tool := range toolAndContent.ToolList {
		toolname := tool.Function.Name
		var outStr string
		var executedCommand string
		directCompleteTool := isDirectCompleteTool(ctx, toolname)
		if !directCompleteTool {
			directCompleteOnly = false
		}

		functl, flag := tools.Getregistry().Get(toolname)

		outStr = fmt.Sprintf("开始调用工具%s, 参数是%s", toolname, tool.Function.Arguments)
		globalchannel.SendAssistantMessageOnce(ctx, fmt.Sprintf("%s", outStr))
		logging.Info("%s", outStr)

		start := time.Now()
		var (
			str string
			err error
		)
		if flag {
			if toolname == bashfunction.CommandToolName {
				cmdLine, perr := bashfunction.ParseCommandFromToolArgs(tool.Function.Arguments)
				executedCommand = cmdLine
				if perr != nil {
					str, err = "", perr
				} else if verr := bashfunction.ValidateCommand(cmdLine); verr != nil {
					str, err = "", fmt.Errorf("command validation failed: %w", verr)
				} else if apprErr := shellapproval.WaitApprove(ctx, chatId, utils.GenerateMessageID(), cmdLine); apprErr != nil {
					str, err = "", apprErr
				} else {
					str, err = functl.Execute(ctx, tool.Function.Arguments)
				}
			} else {
				str, err = functl.Execute(ctx, tool.Function.Arguments)
			}
		} else if _, ok := mcpbridge.ResolveDynamicTool(toolname); ok {
			str, err = mcpbridge.ExecuteDynamicTool(ctx, toolname, tool.Function.Arguments)
		} else {
			directCompleteOnly = false
			outStr = fmt.Sprintf("工具%s不存在", toolname)
			memory.AddToolMessage(chatId, tool.ID, outStr)
			logging.Error("%s", outStr)
			break
		}
		elapsed := time.Since(start)
		if err != nil {
			switch {
			case errors.Is(err, shellapproval.ErrDenied):
				outStr = fmt.Sprintf("工具%s未执行: 用户拒绝运行该 shell 命令 (elapsed=%s)", toolname, elapsed)
			case errors.Is(err, context.Canceled):
				outStr = fmt.Sprintf("工具%s未执行: 任务已中断 (elapsed=%s)", toolname, elapsed)
			default:
				outStr = fmt.Sprintf("工具%s执行失败: %v (elapsed=%s)", toolname, err, elapsed)
			}
			if shouldReflectSkillPreflight(toolname, executedCommand, err, str) {
				toolCompletePrompt = buildSkillPreflightRecoveryPrompt(executedCommand, err)
			}
			directCompleteOnly = false
			logging.Error("%s", outStr)
			memory.AddToolMessage(chatId, tool.ID, outStr)
			globalchannel.SendAssistantMessageOnce(ctx, fmt.Sprintf("%s", outStr))

			break
		}

		// resultPreview := truncateForLog(str, 1200)
		outStr = fmt.Sprintf("工具%s执行成功 (elapsed=%s): %s", toolname, elapsed, str)
		displayStr := formatToolSuccessForDisplay(toolname, elapsed, str, directCompleteTool)
		logging.Info("%s", outStr)
		memory.AddToolMessage(chatId, tool.ID, outStr)
		globalchannel.SendAssistantMessageOnce(ctx, displayStr)
		toolResults = append(toolResults, outStr)
	}

	if directCompleteOnly {
		summary := buildDirectToolCompletionSummary(toolResults)
		if summary != "" {
			memory.CompactLatestToolRun(chatId, summary)
		}
		logging.Info("工具执行完成: MCP/skill 直接完成型工具已直接输出，跳过二次 LLM 总结")
		return
	}

	if a.taskLoopTimes >= 0 {
		logging.Info("工具执行完成,继续请求模型生成最终回复")
		a.taskLoopTimes--
		backInfo, err := a.handleToolCompleteChat(ctx, toolCompletePrompt)
		if err != nil {
			logging.Error("继续请求模型生成最终回复失败: %v", err)
			return
		}

		// 如果 backInfo 包含 needToolToics 字段，则按请求加载额外工具 topic 后继续对话。
		if needToolTopics, ok := utils.GetNeedToolToicsFromPayload(backInfo); ok {
			for _, topic := range needToolTopics {
				logging.Info("模型请求补充工具话题: %s", topic)
				nextCtx := context.WithValue(ctx, utils.ToolTopicToLoad, topic)

				if _, err := a.handleToolCompleteChat(nextCtx, fmt.Sprintf("已按你的要求加载%s相关工具，请继续。返回规则保持不变：如果仍需要调用已加载工具，请直接使用原生 tool-call；如果缺少工具或不再需要工具，必须只返回包含 needToolToics、content、summaryfornextllm 的 JSON。", topic)); err != nil {
					logging.Error("加载工具话题 %s 后继续请求失败: %v", topic, err)
					continue
				}
				return
			}

			logging.Warn("模型请求了额外工具话题，但未解析出可用 topic: %v", backInfo)
		}
	}
	logging.Info("工具执行完成,或者达到最大循环次数,结束工具执行")

}

func shouldReflectSkillPreflight(toolName, command string, err error, output string) bool {
	if toolName != bashfunction.CommandToolName || strings.TrimSpace(command) == "" || err == nil {
		return false
	}
	text := strings.ToLower(err.Error() + "\n" + output)
	return strings.Contains(text, "command not found") ||
		strings.Contains(text, "executable file not found") ||
		strings.Contains(text, "exit code 127") ||
		strings.Contains(text, "no such file or directory")
}

func buildSkillPreflightRecoveryPrompt(command string, err error) string {
	return fmt.Sprintf(`上一个 shell 命令执行失败，疑似缺少 skill 前置依赖。

失败命令：%s
失败原因：%v

请先反思这是不是因为没有按相关 SKILL.md 的 Installation / Requirements / metadata.openclaw.requires 做前置准备。

恢复规则：
- 如果任务明显来自某个 skill，先调用 read_openclaw_skill 读取对应 SKILL.md（如果本轮还没读过）。
- 检查 SKILL.md 正文的 Installation、Requirements、Setup、Usage，以及 frontmatter 中 metadata.openclaw.requires / metadata.openclaw.install。
- 如果缺少的是可安装 CLI 或运行时依赖，优先用 execute_command 安装或初始化依赖，然后重试原命令。
- 如果缺少的是环境变量、账号登录、API key、权限，或者安装命令有破坏性/不确定，向用户简短说明需要用户处理什么。
- 不要立刻把 command not found 当作最终失败；先完成 skill preflight。

如果仍需要调用当前已经加载的工具：不要返回 JSON，直接使用模型原生 tool-call 调用工具。

如果当前缺少必要工具，或者已经不需要再调用工具，必须只返回一个合法 JSON 对象，不要使用 Markdown，不要输出额外文字：
{
  "needToolToics": [],
  "content": "给用户看的简略回复",
  "summaryfornextllm": "供下一轮 LLM 使用的压缩摘要"
}

字段规则：
- needToolToics：如果缺少工具，填入需要补充加载的 topic 数组；如果不缺工具或任务已完成，填 []。topic 必须是精确的 topic 名称本身。
- content：给用户看的最终回复或缺少工具说明，尽量简略。
- summaryfornextllm：必须填写，用不超过300字压缩本轮关键信息。`, command, err)
}

func isDirectCompleteTool(ctx context.Context, toolName string) bool {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return false
	}
	if _, ok := mcpbridge.ResolveDynamicTool(toolName); ok {
		return true
	}

	switch toolName {
	case "call_mcp_tool", "list_mcp_tools", "register_mcp_from_hub":
		return true
	case "install_openclaw_skill_from_market", "baidu_search":
		return true
	case "read_openclaw_skill":
		return false
	}

	toolSource, _ := ctx.Value(utils.ToolSourceToLoad).(string)
	toolTopic, _ := ctx.Value(utils.ToolTopicToLoad).(string)
	if strings.EqualFold(strings.TrimSpace(toolSource), utils.ToolSourceMCP) || strings.EqualFold(strings.TrimSpace(toolTopic), utils.ToolTopicMCP) {
		return strings.Contains(strings.ToLower(toolName), "mcp")
	}

	return false
}

func buildDirectToolCompletionSummary(results []string) string {
	cleaned := make([]string, 0, len(results))
	for _, result := range results {
		result = strings.TrimSpace(result)
		if result == "" {
			continue
		}
		cleaned = append(cleaned, truncateForLog(result, 1200))
	}
	if len(cleaned) == 0 {
		return ""
	}
	return "【工具执行结果，已直接展示给用户】\n" + strings.Join(cleaned, "\n\n")
}

func formatToolSuccessForDisplay(toolName string, elapsed time.Duration, result string, markdownJSON bool) string {
	if markdownJSON {
		if md, ok := jsonToolResultToMarkdown(result); ok {
			return fmt.Sprintf("### 工具 %s 执行成功\n\n- 耗时：`%s`\n\n%s", toolName, elapsed, md)
		}
	}
	return fmt.Sprintf("工具%s执行成功 (elapsed=%s): %s", toolName, elapsed, result)
}

func jsonToolResultToMarkdown(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	var value interface{}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", false
	}
	md := strings.TrimSpace(markdownFromJSONValue(value, 0))
	if md == "" {
		return "", false
	}
	return md, true
}

func markdownFromJSONValue(value interface{}, depth int) string {
	switch v := value.(type) {
	case map[string]interface{}:
		return markdownFromJSONObject(v, depth)
	case []interface{}:
		return markdownFromJSONArray(v, depth)
	case string:
		return strings.TrimSpace(v)
	case json.Number:
		return v.String()
	case bool:
		if v {
			return "true"
		}
		return "false"
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

func markdownFromJSONObject(obj map[string]interface{}, depth int) string {
	var b strings.Builder
	if text := markdownFromMCPContent(obj["content"]); text != "" {
		b.WriteString(text)
	}
	if structured, ok := obj["structured_content"]; ok {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("**结构化结果**\n\n")
		b.WriteString(markdownFromJSONValue(structured, depth+1))
	}

	known := map[string]struct{}{
		"content":            {},
		"structured_content": {},
		"raw":                {},
	}
	keys := orderedJSONKeys(obj, "server_label", "name", "is_error")
	for _, key := range keys {
		if _, skip := known[key]; skip {
			continue
		}
		val := strings.TrimSpace(markdownInlineJSONValue(obj[key]))
		if val == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(fmt.Sprintf("- **%s**: %s", key, val))
	}
	if b.Len() == 0 {
		for _, key := range orderedJSONKeys(obj) {
			if key == "raw" {
				continue
			}
			val := strings.TrimSpace(markdownFromJSONValue(obj[key], depth+1))
			if val == "" {
				continue
			}
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(fmt.Sprintf("- **%s**: %s", key, val))
		}
	}
	return b.String()
}

func markdownFromJSONArray(items []interface{}, depth int) string {
	lines := make([]string, 0, len(items))
	for idx, item := range items {
		md := strings.TrimSpace(markdownFromJSONValue(item, depth+1))
		if md == "" {
			continue
		}
		if strings.Contains(md, "\n") {
			lines = append(lines, fmt.Sprintf("%d. %s", idx+1, strings.ReplaceAll(md, "\n", "\n   ")))
		} else {
			lines = append(lines, fmt.Sprintf("%d. %s", idx+1, md))
		}
	}
	return strings.Join(lines, "\n")
}

func markdownFromMCPContent(content interface{}) string {
	items, ok := content.([]interface{})
	if !ok {
		return strings.TrimSpace(markdownFromJSONValue(content, 0))
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		obj, ok := item.(map[string]interface{})
		if !ok {
			parts = append(parts, strings.TrimSpace(markdownFromJSONValue(item, 0)))
			continue
		}
		if text, ok := obj["text"].(string); ok && strings.TrimSpace(text) != "" {
			parts = append(parts, strings.TrimSpace(text))
			continue
		}
		parts = append(parts, strings.TrimSpace(markdownFromJSONValue(obj, 0)))
	}
	return strings.Join(nonEmptyStrings(parts), "\n\n")
}

func markdownInlineJSONValue(value interface{}) string {
	switch v := value.(type) {
	case map[string]interface{}, []interface{}:
		data, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return "`" + string(data) + "`"
	default:
		return markdownFromJSONValue(v, 0)
	}
}

func orderedJSONKeys(obj map[string]interface{}, preferred ...string) []string {
	seen := map[string]struct{}{}
	keys := make([]string, 0, len(obj))
	for _, key := range preferred {
		if _, ok := obj[key]; ok {
			keys = append(keys, key)
			seen[key] = struct{}{}
		}
	}
	rest := make([]string, 0, len(obj))
	for key := range obj {
		if _, ok := seen[key]; ok {
			continue
		}
		rest = append(rest, key)
	}
	sort.Strings(rest)
	return append(keys, rest...)
}

func nonEmptyStrings(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}
