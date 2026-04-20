package dispatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"leiAgent/internal/tools"
	"leiAgent/logging"
	"leiAgent/utils"
)

func (d *Dispatcher) prepareToolExecutionContext(ctx context.Context, blueprint ExecutionBlueprint) context.Context {
	directives := append([]string(nil), blueprint.SystemDirectives...)
	for _, step := range blueprint.PreSteps {
		if !strings.EqualFold(strings.TrimSpace(step.Kind), "tool") {
			continue
		}
		if strings.TrimSpace(step.ToolName) == "" {
			continue
		}

		toolImpl, ok := tools.Getregistry().Get(step.ToolName)
		if !ok {
			logging.Warn("blueprint prestep tool not found: %s", step.ToolName)
			continue
		}

		out, err := toolImpl.Execute(ctx, "{}")
		if err != nil {
			logging.Warn("blueprint prestep tool failed: %s err=%v", step.ToolName, err)
			continue
		}

		if strings.EqualFold(step.ToolName, "get_current_time") {
			currentTime := parseCurrentTimeAnchor(out)
			if currentTime != "" {
				directives = append(directives,
					fmt.Sprintf("Execution context: current server time is %s.", currentTime),
					fmt.Sprintf("Use %s as the authoritative 'now' for this request.", currentTime),
				)
				ctx = context.WithValue(ctx, utils.FreshnessTimeAnchorString, currentTime)
			} else {
				directives = append(directives,
					fmt.Sprintf("Execution context: current server time tool returned: %s", strings.TrimSpace(out)),
				)
			}
		}
	}

	if len(directives) > 0 {
		ctx = mergeExtraSystemMessages(ctx, directives...)
	}
	return ctx
}

func parseCurrentTimeAnchor(raw string) string {
	payload := map[string]interface{}{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return ""
	}
	if currentTime, _ := payload["current_time"].(string); strings.TrimSpace(currentTime) != "" {
		return strings.TrimSpace(currentTime)
	}
	return ""
}
