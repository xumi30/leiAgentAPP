package agent

import (
	"context"
	"fmt"
	"leiAgent/internal/tools/bashfunction"
	"leiAgent/utils"
	"strings"
	"testing"
)

func TestShouldReflectSkillPreflightForMissingCommand(t *testing.T) {
	if !shouldReflectSkillPreflight(
		bashfunction.CommandToolName,
		"agent-browser open https://example.com",
		fmt.Errorf("command failed with exit code 127: bash: agent-browser: command not found"),
		"",
	) {
		t.Fatal("expected missing command to trigger skill preflight reflection")
	}
}

func TestShouldReflectSkillPreflightIgnoresNonShellTool(t *testing.T) {
	if shouldReflectSkillPreflight(
		"baidu_search",
		"agent-browser open https://example.com",
		fmt.Errorf("command not found"),
		"",
	) {
		t.Fatal("non-shell tools should not trigger shell skill preflight reflection")
	}
}

func TestIsDirectCompleteTool(t *testing.T) {
	ctx := context.Background()
	if !isDirectCompleteTool(ctx, "call_mcp_tool") {
		t.Fatal("call_mcp_tool should be direct-complete")
	}
	if !isDirectCompleteTool(ctx, "install_openclaw_skill_from_market") {
		t.Fatal("skill install tool should be direct-complete")
	}
	if isDirectCompleteTool(ctx, "read_openclaw_skill") {
		t.Fatal("read_openclaw_skill should continue through LLM")
	}
	if isDirectCompleteTool(ctx, bashfunction.CommandToolName) {
		t.Fatal("local shell tool should not be direct-complete")
	}
}

func TestIsDirectCompleteToolForMCPContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), utils.ToolSourceToLoad, utils.ToolSourceMCP)
	if !isDirectCompleteTool(ctx, "list_mcp_tools") {
		t.Fatal("mcp context mcp tool should be direct-complete")
	}
}

func TestBuildDirectToolCompletionSummary(t *testing.T) {
	summary := buildDirectToolCompletionSummary([]string{" result one ", strings.Repeat("x", 1300)})
	if !strings.Contains(summary, "已直接展示给用户") {
		t.Fatalf("summary missing direct display marker: %q", summary)
	}
	if !strings.Contains(summary, "...(truncated)") {
		t.Fatalf("summary should truncate long tool output: %q", summary)
	}
}

func TestFormatToolSuccessForDisplayJSON(t *testing.T) {
	raw := `{
		"server_label": "demo",
		"name": "lookup",
		"content": [{"type":"text","text":"找到 2 条结果。"}],
		"structured_content": {"count": 2, "ok": true}
	}`
	display := formatToolSuccessForDisplay("mcp_demo_lookup", 10, raw, true)
	if !strings.Contains(display, "### 工具 mcp_demo_lookup 执行成功") {
		t.Fatalf("display missing markdown title: %q", display)
	}
	if !strings.Contains(display, "找到 2 条结果。") {
		t.Fatalf("display missing text content: %q", display)
	}
	if !strings.Contains(display, "**结构化结果**") {
		t.Fatalf("display missing structured section: %q", display)
	}
	if strings.Contains(display, "{\n") {
		t.Fatalf("display should not be raw json: %q", display)
	}
}

func TestFormatToolSuccessForDisplayNonJSON(t *testing.T) {
	display := formatToolSuccessForDisplay("mcp_demo", 10, "plain output", true)
	if !strings.Contains(display, "plain output") || strings.Contains(display, "###") {
		t.Fatalf("non-json output should fall back to plain message: %q", display)
	}
}
