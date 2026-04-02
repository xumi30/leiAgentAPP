package timeFunctions

import (
	"context"
	"fmt"
	"leiAgent/internal/tools"
	"time"
)

// TimeTool implements the Tool interface for getting the current time
type CurrentTimeTool struct{}

func NewCurrentTimeTool() tools.Tool {
	return &CurrentTimeTool{}
}

// Name returns the name of the tool
func (t *CurrentTimeTool) Name() string {
	return "get_current_time"
}

// Description returns a description of what the tool does
func (t *CurrentTimeTool) Description() string {
	return "Tool for getting the current real-time date and time from the server. " +
		"MUST use this tool when user asks about current time, today's date, " +
		"or any time-related questions requiring real-time information. " +
		"Returns date, time, weekday, lunar date, and holiday information." +
		"只要对话里涉及时间的要应该先调用这个接口获取当前时间，再根据用户需求进行回复"
}

// Parameters returns the parameters that the tool accepts
func (t *CurrentTimeTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
		"required":   []string{},
	}
}

// Run executes the tool with the given input
func (t *CurrentTimeTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

// Execute executes the tool with the given arguments
func (t *CurrentTimeTool) Execute(ctx context.Context, args string) (string, error) {
	now := time.Now()

	// 获取星期几
	weekdays := []string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}
	weekday := weekdays[now.Weekday()]

	// 获取节日信息
	holiday := getHoliday(now)

	// 构建详细的时间信息
	result := fmt.Sprintf("当前时间: %s\n", now.Format("2006-01-02 15:04:05"))
	result += fmt.Sprintf("星期: %s\n", weekday)
	result += fmt.Sprintf("农历: %s\n", getLunarDate(now))
	if holiday != "" {
		result += fmt.Sprintf("节日: %s\n", holiday)
	}

	return result, nil
}
