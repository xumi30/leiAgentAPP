package dispatcher

import (
	"testing"

	"leiAgent/utils"
)

func TestAnalyzeTaskDetectsFreshExplicitMCPRequest(t *testing.T) {
	intent := &Intention{Intent: utils.ToolModeString}

	profile := AnalyzeTask("使用 MCP 搜索今天的最新 AI 新闻", intent)

	if !profile.RequiresTools || !profile.RequiresFreshness || !profile.RequiresTimeAnchor {
		t.Fatalf("unexpected execution requirements: %+v", profile)
	}
	if !profile.RequiresSearch || !profile.ExplicitMCP || profile.Domain != "search" {
		t.Fatalf("unexpected routing profile: %+v", profile)
	}
}

func TestBuildExecutionBlueprintAnchorsFreshToolRequests(t *testing.T) {
	profile := TaskProfile{RequiresFreshness: true, ExplicitMCP: true}
	intent := &Intention{Intent: utils.ToolModeString}

	blueprint := BuildExecutionBlueprint(profile, intent)

	if blueprint.ToolSource != utils.ToolSourceMCP {
		t.Fatalf("ToolSource = %q, want %q", blueprint.ToolSource, utils.ToolSourceMCP)
	}
	if blueprint.ToolTopic != utils.ToolTopicSearch {
		t.Fatalf("ToolTopic = %q, want %q", blueprint.ToolTopic, utils.ToolTopicSearch)
	}
	if len(blueprint.PreSteps) != 1 || blueprint.PreSteps[0].ToolName != "get_current_time" {
		t.Fatalf("PreSteps = %+v, want get_current_time", blueprint.PreSteps)
	}
	if !blueprint.Verification.RequireExplicitAsOf {
		t.Fatal("RequireExplicitAsOf = false, want true")
	}
}

func TestBuildExecutionBlueprintPreservesExplicitRouting(t *testing.T) {
	profile := TaskProfile{RequiresFreshness: true}
	intent := &Intention{
		Intent:     utils.ToolModeString,
		ToolSource: "local",
		ToolTopic:  "天气",
	}

	blueprint := BuildExecutionBlueprint(profile, intent)

	if blueprint.ToolSource != intent.ToolSource || blueprint.ToolTopic != intent.ToolTopic {
		t.Fatalf("explicit routing was overwritten: %+v", blueprint)
	}
}
