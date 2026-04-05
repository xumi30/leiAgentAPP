package memotool

import (
	"context"
	"encoding/json"
	"fmt"
	"leiAgent/internal/memo"
	"leiAgent/internal/tools"
)

// MemoWriteTool writes to the app memo file (same as the UI 备忘录).
type MemoWriteTool struct{}

func NewMemoWriteTool() tools.Tool {
	return &MemoWriteTool{}
}

func (t *MemoWriteTool) Name() string {
	return "memo_write"
}

func (t *MemoWriteTool) Description() string {
	return `Write or update the user's in-app memo / diary file (Markdown). Same storage as the 备忘录 panel (data/memo.md).
Use for travel plans, todos, notes the user asked to keep. Prefer structured Markdown (headings, lists).
For the sidebar calendar "memo dot", include a date like YYYY-MM-DD in section_title or in the first lines of content (or use default append heading which includes the date).
mode "append" adds a new dated section; "replace" overwrites the whole memo (use only when the user explicitly wants to clear or replace everything).
Args JSON: {"content": "...", "mode": "append"|"replace", "section_title": "optional heading for append"}`
}

func (t *MemoWriteTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Markdown text to save.",
			},
			"mode": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"append", "replace"},
				"description": "append (default): add a section; replace: overwrite entire memo.",
			},
			"section_title": map[string]interface{}{
				"type":        "string",
				"description": "Optional section heading when mode is append; default is local date-time.",
			},
		},
		"required": []string{"content"},
	}
}

func (t *MemoWriteTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

func (t *MemoWriteTool) Execute(ctx context.Context, args string) (string, error) {
	var params struct {
		Content       string `json:"content"`
		Mode          string `json:"mode"`
		SectionTitle  string `json:"section_title"`
		SectionTitle2 string `json:"sectionTitle"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("invalid JSON args: %w", err)
	}
	if params.Content == "" {
		return "", fmt.Errorf("content is required")
	}
	mode := params.Mode
	if mode == "" {
		mode = "append"
	}
	title := params.SectionTitle
	if title == "" {
		title = params.SectionTitle2
	}
	switch mode {
	case "replace":
		if err := memo.WriteAll(params.Content); err != nil {
			return "", err
		}
		return fmt.Sprintf("Memo replaced: %s", memo.Path()), nil
	case "append":
		if err := memo.AppendBlock(title, params.Content); err != nil {
			return "", err
		}
		return fmt.Sprintf("Memo appended: %s", memo.Path()), nil
	default:
		return "", fmt.Errorf("mode must be append or replace, got %q", mode)
	}
}

func (t *MemoWriteTool) Results() map[string]interface{} {
	return map[string]interface{}{
		"type":        "string",
		"description": "Confirmation with memo file path.",
	}
}
