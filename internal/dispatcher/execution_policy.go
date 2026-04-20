package dispatcher

import (
	"strings"

	"leiAgent/utils"
)

type BlueprintPreStep struct {
	Kind     string
	ToolName string
}

type VerificationPolicy struct {
	RequireExplicitAsOf bool
}

type ExecutionBlueprint struct {
	Mode             string
	ToolSource       string
	ToolTopic        string
	PreSteps         []BlueprintPreStep
	SystemDirectives []string
	Verification     VerificationPolicy
}

func BuildExecutionBlueprint(profile TaskProfile, intent *Intention) ExecutionBlueprint {
	blueprint := ExecutionBlueprint{}
	if intent == nil {
		return blueprint
	}

	blueprint.Mode = strings.TrimSpace(intent.Intent)
	blueprint.ToolSource = strings.TrimSpace(intent.ToolSource)
	blueprint.ToolTopic = strings.TrimSpace(intent.ToolTopic)

	if profile.RequiresFreshness && strings.EqualFold(strings.TrimSpace(intent.Intent), utils.ToolModeString) {
		if blueprint.ToolSource == "" {
			if profile.ExplicitMCP {
				blueprint.ToolSource = utils.ToolSourceMCP
			} else {
				blueprint.ToolSource = utils.ToolSourceMixed
			}
		}
		if blueprint.ToolTopic == "" {
			blueprint.ToolTopic = utils.ToolTopicSearch
		}

		blueprint.PreSteps = append(blueprint.PreSteps, BlueprintPreStep{
			Kind:     "tool",
			ToolName: "get_current_time",
		})
		blueprint.SystemDirectives = append(blueprint.SystemDirectives,
			"This request is freshness-sensitive.",
			"You will receive a current server-time anchor in the execution context.",
			"Before answering, use that time anchor to derive date and year constraints.",
			"Do not guess a year or date that differs from the provided current-time anchor.",
			"When summarizing search results, prefer the newest reliable items and explicitly mention the as-of date.",
		)
		blueprint.Verification.RequireExplicitAsOf = true
	}

	return blueprint
}
