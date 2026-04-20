package dispatcher

import (
	"strings"
)

type TaskProfile struct {
	UserGoal           string
	RequiresTools      bool
	RequiresFreshness  bool
	RequiresTimeAnchor bool
	RequiresSearch     bool
	ExplicitMCP        bool
	Domain             string
}

func AnalyzeTask(message string, intent *Intention) TaskProfile {
	goal := strings.TrimSpace(message)
	lower := strings.ToLower(goal)

	profile := TaskProfile{
		UserGoal:      goal,
		RequiresTools: intent != nil && strings.EqualFold(strings.TrimSpace(intent.Intent), "TOOL"),
		ExplicitMCP:   strings.Contains(lower, "mcp"),
	}

	freshnessKeywords := []string{
		"最新", "当前", "今天", "现在", "最近", "截至", "实时", "最新情况", "最新消息", "最新动态",
	}
	for _, keyword := range freshnessKeywords {
		if strings.Contains(goal, keyword) || strings.Contains(lower, keyword) {
			profile.RequiresFreshness = true
			profile.RequiresTimeAnchor = true
			break
		}
	}

	searchKeywords := []string{
		"搜", "搜索", "查", "查询", "新闻", "消息", "动态", "情况", "资讯", "报道",
	}
	for _, keyword := range searchKeywords {
		if strings.Contains(goal, keyword) || strings.Contains(lower, keyword) {
			profile.RequiresSearch = true
			break
		}
	}

	switch {
	case strings.Contains(goal, "战争") || strings.Contains(goal, "冲突") || strings.Contains(goal, "局势"):
		profile.Domain = "news"
	case profile.RequiresSearch:
		profile.Domain = "search"
	default:
		profile.Domain = "general"
	}

	return profile
}
