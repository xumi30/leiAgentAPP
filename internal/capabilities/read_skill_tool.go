package capabilities

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"leiAgent/internal/openclawskill"
	"leiAgent/internal/tools"
	"leiAgent/utils"
	"os"
	"path/filepath"
	"strings"
)

const maxSkillReadBytes = 96 * 1024

type ReadSkillTool struct{}

func NewReadSkillTool() tools.Tool {
	return &ReadSkillTool{}
}

func (t *ReadSkillTool) Name() string {
	return SkillReaderToolName
}

func (t *ReadSkillTool) Description() string {
	return "Read an installed OpenClaw/ClawHub skill's SKILL.md instructions. Use this only after the skill catalog indicates a likely match."
}

func (t *ReadSkillTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"skill": map[string]interface{}{
				"type":        "string",
				"description": "Skill name or installed folder name, for example baidu-search or 百度搜索.",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Optional installed skill directory path or SKILL.md path under the configured skills root.",
			},
		},
	}
}

func (t *ReadSkillTool) Execute(ctx context.Context, args string) (string, error) {
	_ = ctx
	var params struct {
		Skill string `json:"skill"`
		Path  string `json:"path"`
	}
	if strings.TrimSpace(args) != "" {
		if err := json.Unmarshal([]byte(args), &params); err != nil {
			return "", fmt.Errorf("failed to parse arguments: %w", err)
		}
	}
	skillPath, err := resolveSkillFile(params.Skill, params.Path)
	if err != nil {
		return "", err
	}
	file, err := os.Open(skillPath)
	if err != nil {
		return "", fmt.Errorf("read skill failed: %w", err)
	}
	defer file.Close()

	limited := io.LimitReader(file, maxSkillReadBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("read skill failed: %w", err)
	}
	truncated := len(data) > maxSkillReadBytes
	if truncated {
		data = data[:maxSkillReadBytes]
	}
	result := map[string]interface{}{
		"path":      skillPath,
		"content":   string(data),
		"truncated": truncated,
	}
	out, _ := json.MarshalIndent(result, "", "  ")
	return string(out), nil
}

func (t *ReadSkillTool) Results() map[string]interface{} {
	return map[string]interface{}{
		"type":        "object",
		"description": "The SKILL.md path, content, and whether the content was truncated.",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type": "string",
			},
			"content": map[string]interface{}{
				"type": "string",
			},
			"truncated": map[string]interface{}{
				"type": "boolean",
			},
		},
	}
}

func (t *ReadSkillTool) SimpleInfo() map[string]string {
	return utils.SimpleInfoMap(utils.ToolTopicMCP, "按需读取已安装 skill 的 SKILL.md 指令，供 LLM 选择并执行 skill 工作流。")
}

func resolveSkillFile(skillName, rawPath string) (string, error) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath != "" {
		return cleanSkillFile(rawPath)
	}
	skillName = strings.TrimSpace(skillName)
	if skillName == "" {
		return "", fmt.Errorf("skill or path is required")
	}
	info, ok := openclawskill.Find(skillName)
	if !ok {
		return "", fmt.Errorf("skill %q is not installed", skillName)
	}
	return cleanSkillFile(filepath.Join(info.Path, "SKILL.md"))
}

func cleanSkillFile(rawPath string) (string, error) {
	target := filepath.Clean(rawPath)
	if !filepath.IsAbs(target) {
		target = filepath.Join(openclawskill.SkillsRoot(), target)
	}
	if filepath.Base(target) != "SKILL.md" {
		target = filepath.Join(target, "SKILL.md")
	}
	root, err := filepath.Abs(openclawskill.SkillsRoot())
	if err != nil {
		return "", err
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return "", fmt.Errorf("skill path must be under %s", root)
	}
	if st, err := os.Stat(target); err != nil {
		return "", fmt.Errorf("skill file not found: %w", err)
	} else if st.IsDir() {
		return "", fmt.Errorf("skill path is a directory, expected SKILL.md")
	}
	return target, nil
}
