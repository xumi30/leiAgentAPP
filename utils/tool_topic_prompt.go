package utils

import (
	"fmt"
	"strings"
)

var ToolTopicDescriptions = map[string]string{
	ToolTopicTime:    "时间相关工具。适合当前时间、日期换算、节气农历、某一天是什么时间信息。",
	ToolTopicSearch:  "搜索相关工具。适合搜索网页信息、百科、天气、地理坐标、金融行情、检索资源线索。",
	ToolTopicBrowser: "浏览器网页操作工具。适合打开网页、点击、输入、抓取页面内容、截图、在真实网页里继续交互。",
	ToolTopicFiles:   "文件与下载工具。适合写文件、分块写大文件、列目录、读写 workspace 文件、以及当用户已经提供直接下载 URL 时把文件下载到 workspace。",
	ToolTopicSystem:  "系统 bash/cmd 命令工具。适合执行安全的本地命令，例如 curl、解压、查看环境、运行程序、检查系统状态。",
	ToolTopicCrontab: "定时任务工具。适合创建/更新/删除/列出定时任务，支持一次性任务与周期任务（RRULE/cron），并计算下一次触发时间。",
	ToolTopicWriting: "写作生成工具。适合小说、章节、大纲、长文创作与续写。",
	ToolTopicMCP:     "MCP 外部工具。适合调用外部 MCP 服务提供的扩展能力。",
}

func ToolTopicsPromptText() string {
	parts := make([]string, 0, len(ToolTopics))
	for _, topic := range ToolTopics {
		desc := strings.TrimSpace(ToolTopicDescriptions[topic])
		if desc == "" {
			parts = append(parts, topic)
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %s", topic, desc))
	}
	return strings.Join(parts, "; ")
}
