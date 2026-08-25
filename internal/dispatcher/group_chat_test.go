package dispatcher

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"leiAgent/internal/memory"
	"leiAgent/internal/provider/openaistyle"
	"leiAgent/utils"
)

func TestGroupReplyOrderToolmanFirstAndOneRandomRole(t *testing.T) {
	order := defaultGroupReplyOrder(
		[]string{"", defaultAssistantAgentID, "agent-a", "agent-a", "agent-b"},
		func(count int) int {
			if count != 2 {
				t.Fatalf("expected two eligible roles, got %d", count)
			}
			return 1
		},
	)

	want := []string{defaultAssistantAgentID, "agent-b"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("unexpected reply order: got %v want %v", order, want)
	}
}

func TestGroupReplyOrderAllowsToolmanOnly(t *testing.T) {
	order := defaultGroupReplyOrder([]string{"", defaultAssistantAgentID}, func(int) int {
		t.Fatal("picker must not run without an eligible role")
		return 0
	})

	want := []string{defaultAssistantAgentID}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("unexpected reply order: got %v want %v", order, want)
	}
}

func TestMentionedReplyAgentSingleTargetDoesNotInsertToolman(t *testing.T) {
	got := mentionedReplyAgent([]string{"agent-xiaoran"}, func(count int) int {
		if count != 1 {
			t.Fatalf("expected one mentioned role, got %d", count)
		}
		return 0
	})
	if got != "agent-xiaoran" {
		t.Fatalf("unexpected mentioned reply agent: %q", got)
	}
}

func TestMentionedReplyAgentMultipleTargetsPicksOne(t *testing.T) {
	got := mentionedReplyAgent([]string{"agent-a", "agent-a", "agent-b"}, func(count int) int {
		if count != 2 {
			t.Fatalf("expected two unique mentioned roles, got %d", count)
		}
		return 1
	})
	if got != "agent-b" {
		t.Fatalf("unexpected mentioned reply agent: %q", got)
	}
}

func TestWithGroupToolmanPolicyPreservesExistingInstructions(t *testing.T) {
	ctx := context.WithValue(context.Background(), utils.ExtraSystemMessagesString, []string{"existing"})
	ctx = withGroupToolmanPolicy(ctx)

	if agentID, _ := ctx.Value(utils.AgentID).(string); agentID != defaultAssistantAgentID {
		t.Fatalf("unexpected toolman agent id: %q", agentID)
	}
	messages, _ := ctx.Value(utils.ExtraSystemMessagesString).([]string)
	if len(messages) != 2 || messages[0] != "existing" || messages[1] != groupToolmanPrompt {
		t.Fatalf("unexpected system messages: %v", messages)
	}
}

func TestLatestAssistantContentFindsToolmanAnswer(t *testing.T) {
	messages := []*memory.Message{
		{Role: memory.MessageRoleUser, Content: "问题"},
		{Role: memory.MessageRoleAssistant, Content: "旧答案"},
		{Role: memory.MessageRoleTool, Content: "工具结果"},
		{Role: memory.MessageRoleAssistant, Content: " 工具人本轮答案 "},
	}
	if got := latestAssistantContent(messages); got != "工具人本轮答案" {
		t.Fatalf("unexpected latest assistant content: %q", got)
	}
}

func TestBuildGroupRoleMessagesLabelsToolmanAnswerAsAuthority(t *testing.T) {
	messages := buildGroupRoleMessages("policy", "这是真的吗？", "这是工具人确认的信息")
	if len(messages) != 2 {
		t.Fatalf("expected two focused messages, got %d", len(messages))
	}
	if messages[0].Role != openaistyle.RoleSystem || messages[1].Role != openaistyle.RoleUser {
		t.Fatalf("unexpected message roles: %+v", messages)
	}
	content, _ := messages[1].Content.(string)
	if content == "" || !containsAll(content, "工具人本轮权威信息", "这是工具人确认的信息") {
		t.Fatalf("toolman authority was not explicit: %q", content)
	}
}

func TestBuildMentionedRoleMessagesUsesRecentContextWithoutToolmanAuthority(t *testing.T) {
	messages := buildMentionedRoleMessages(
		"persona",
		[]IntentContextMessage{
			{Role: openaistyle.RoleAssistant, Content: "上一条讨论"},
			{Role: openaistyle.RoleTool, Content: "内部工具结果"},
		},
		"@晓染",
	)
	if len(messages) != 3 {
		t.Fatalf("expected system, recent assistant and current user, got %+v", messages)
	}
	if messages[1].Role != openaistyle.RoleAssistant || messages[2].Role != openaistyle.RoleUser {
		t.Fatalf("unexpected message order: %+v", messages)
	}
	content, _ := messages[2].Content.(string)
	if content != "@晓染" || strings.Contains(content, "工具人本轮权威信息") {
		t.Fatalf("mentioned reply was polluted by default group policy: %q", content)
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
