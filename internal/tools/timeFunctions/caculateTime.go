package timeFunctions

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CalculateTimeTool implements the Tool interface for time calculation
type CalculateTimeTool struct{}

func NewCalculateTimeTool() *CalculateTimeTool {
	return &CalculateTimeTool{}
}

// Name returns the name of the tool
func (t *CalculateTimeTool) Name() string {
	return "calculate_time"
}

// Description returns a description of what the tool does
func (t *CalculateTimeTool) Description() string {
	return "Calculates time by adding or subtracting duration from current time. Args format: 'operation:duration' where operation is 'add' or 'subtract', and duration is like '1h30m' (hours and minutes)"
}

// Parameters returns the parameters that the tool accepts
func (t *CalculateTimeTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"operation": map[string]interface{}{
				"type":        "string",
				"description": "The operation to perform: 'add' or 'subtract'",
			},
			"duration": map[string]interface{}{
				"type":        "string",
				"description": "The duration to add or subtract, e.g., '1h30m' for 1 hour and 30 minutes",
			},
		},
		"required": []string{"operation", "duration"},
	}
}

// Run executes the tool with the given input
func (t *CalculateTimeTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

// Execute executes the tool with the given arguments
// Execute executes the tool with the given arguments
func (t *CalculateTimeTool) Execute(ctx context.Context, args string) (string, error) {
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		// If not JSON, treat the input as the query
		return "", fmt.Errorf("invalid input format: %v", err)
	}
	// Get query parameter
	op, ok := params["operation"].(string)
	if !ok || op == "" {
		return "", fmt.Errorf("query parameter is required")
	}
	duration, ok := params["duration"].(string)
	if !ok || duration == "" {
		return "", fmt.Errorf("query parameter is required")
	}

	// 处理天数单位
	var durationValue time.Duration
	if strings.HasSuffix(duration, "d") {
		daysStr := strings.TrimSuffix(duration, "d")
		days, err := strconv.Atoi(daysStr)
		if err != nil {
			return "", fmt.Errorf("invalid days format: %v", err)
		}
		durationValue = time.Duration(days) * 24 * time.Hour
	} else {
		// Parse the duration string for other units
		var err error
		durationValue, err = time.ParseDuration(duration)
		if err != nil {
			return "", fmt.Errorf("invalid duration format: %v", err)
		}
	}

	// Get current time
	now := time.Now()
	// Calculate the new time based on operation
	var result time.Time
	switch op {
	case "add":
		result = now.Add(durationValue)
	case "subtract":
		result = now.Add(-durationValue)
	default:
		return "", fmt.Errorf("invalid operation: %s. Must be 'add' or 'subtract'", op)
	}
	return fmt.Sprintf("Calculated time: %s", result.Format(time.RFC3339)), nil
}
