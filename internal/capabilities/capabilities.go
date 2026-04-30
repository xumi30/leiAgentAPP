package capabilities

import (
	"encoding/json"
	"fmt"
	mcpbridge "leiAgent/internal/MCP"
	"leiAgent/internal/openclawskill"
	"leiAgent/internal/provider/openaistyle"
	"leiAgent/internal/tools"
	"leiAgent/internal/tools/bashfunction"
	"leiAgent/utils"
	"sort"
	"strings"
)

const SkillReaderToolName = "read_openclaw_skill"

type SkillSimpleInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Ready       bool   `json:"ready"`
}

// BuildSystemPrompt exposes local tools, MCP servers, and installed skills as
// separate capability families. The model still receives concrete function
// schemas separately; this prompt teaches it how to choose between them.
func BuildSystemPrompt(topic, source string) string {
	var b strings.Builder
	b.WriteString(`# Capability Router
You can choose from three independent capability families:
1. Local tools: in-process Go tools. Prefer these when they directly satisfy the request.
2. MCP tools: external tools with standard schemas. Use these when a matching MCP server/tool is available and local tools are not enough.
3. Skills: installed SKILL.md instructions. A skill is not executable by itself; read its SKILL.md only when it clearly matches the task, then follow the instructions using available local tools, MCP tools, or shell commands.

Greedy priority:
- If the user explicitly names a tool, MCP server, or skill, use that family first.
- Otherwise prefer local tools, then MCP, then skills.
- Do not read every skill. Use the skill catalog first, then call read_openclaw_skill for the one matching skill.
- If a skill asks you to run commands, use execute_command and keep commands narrow and safe.
`)
	if strings.TrimSpace(topic) != "" || strings.TrimSpace(source) != "" {
		b.WriteString("\nCurrent intent routing hint:\n")
		if strings.TrimSpace(topic) != "" {
			b.WriteString("- topic: " + topic + "\n")
		}
		if strings.TrimSpace(source) != "" {
			b.WriteString("- preferred source: " + source + "\n")
		}
	}

	if local := localCatalog(topic); local != "" {
		b.WriteString("\n## Local Tools\n")
		b.WriteString(local)
	}
	if mcp := MCPCatalogPrompt(topic); mcp != "" {
		b.WriteString("\n## MCP Tools\n")
		b.WriteString(mcp)
	}
	if skills := SkillsCatalogPrompt(); skills != "" {
		b.WriteString("\n## Installed Skills\n")
		b.WriteString(skills)
	}
	return strings.TrimSpace(b.String())
}

func localCatalog(topic string) string {
	reg := tools.Getregistry()
	var list []tools.Tool
	if strings.TrimSpace(topic) != "" {
		list = reg.ListByTopic(topic)
	}
	if len(list) == 0 {
		list = reg.List()
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name() < list[j].Name() })
	lines := make([]string, 0, len(list))
	for _, tool := range list {
		if tool == nil {
			continue
		}
		desc := strings.TrimSpace(tool.Description())
		if si := tool.SimpleInfo(); si != nil && strings.TrimSpace(si["simpledescription"]) != "" {
			desc = strings.TrimSpace(si["simpledescription"])
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", tool.Name(), oneLine(desc, 220)))
	}
	return strings.Join(lines, "\n")
}

func SkillsCatalogPrompt() string {
	skills := SkillSimpleInfos()
	if len(skills) == 0 {
		return ""
	}
	sort.Slice(skills, func(i, j int) bool {
		return strings.ToLower(skills[i].Name) < strings.ToLower(skills[j].Name)
	})
	lines := make([]string, 0, len(skills)+2)
	lines = append(lines, "<available_skills>")
	for _, skill := range skills {
		name := strings.TrimSpace(skill.Name)
		if name == "" {
			name = strings.TrimSpace(skill.Path)
		}
		desc := oneLine(skill.Description, 260)
		if desc == "" {
			desc = "No description provided."
		}
		lines = append(lines, fmt.Sprintf(`<skill name="%s" path="%s" ready="%t">%s</skill>`,
			xmlEscape(name), xmlEscape(skill.Path), skill.Ready, xmlEscape(desc)))
	}
	lines = append(lines, "</available_skills>")
	lines = append(lines, "To use a skill, call read_openclaw_skill with the skill name or path, then follow that SKILL.md.")
	return strings.Join(lines, "\n")
}

func SkillSimpleInfos() []SkillSimpleInfo {
	scanned := openclawskill.Scan()
	out := make([]SkillSimpleInfo, 0, len(scanned))
	for _, skill := range scanned {
		out = append(out, SkillSimpleInfo{
			Name:        strings.TrimSpace(skill.Name),
			Description: strings.TrimSpace(skill.Description),
			Path:        strings.TrimSpace(skill.Path),
			Ready:       skill.Ready,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func MCPCatalogPrompt(topic string) string {
	infos := mcpbridge.GetMCPSimpleInfos()
	if len(infos) == 0 {
		return ""
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Label < infos[j].Label })
	lines := make([]string, 0, len(infos))
	for _, info := range infos {
		if strings.TrimSpace(topic) != "" && info.Topic != topic {
			continue
		}
		names := strings.Join(info.ToolNames, ", ")
		if names == "" {
			names = fmt.Sprintf("%d cached tools", info.ToolCount)
		}
		lines = append(lines, fmt.Sprintf("- server=%s topic=%s: %s. tools: %s",
			info.Label, info.Topic, oneLine(info.Description, 220), oneLine(names, 260)))
	}
	if len(lines) == 0 {
		return ""
	}
	lines = append(lines, "Concrete MCP function schemas are exposed as mcp_* tool names when this source is loaded.")
	return strings.Join(lines, "\n")
}

func BuildMCPToolsByTopic(topic string) []openaistyle.Tool {
	return mcpbridge.BuildDynamicToolsByTopic(topic)
}

func SupportTools() []openaistyle.Tool {
	out := make([]openaistyle.Tool, 0, 2)
	reg := tools.Getregistry()
	if reader, ok := reg.Get(SkillReaderToolName); ok {
		out = append(out, ToolToOpenAI(reader))
	}
	if bash, ok := reg.Get(bashfunction.CommandToolName); ok {
		out = append(out, ToolToOpenAI(bash))
	}
	return out
}

func ToolToOpenAI(tool tools.Tool) openaistyle.Tool {
	return openaistyle.Tool{
		Type: openaistyle.ToolTypeFunction,
		Function: &openaistyle.Function{
			Name:        tool.Name(),
			Description: tool.Description(),
			Parameters:  tool.Parameters(),
		},
	}
}

func AppendUnique(base []openaistyle.Tool, extra ...openaistyle.Tool) []openaistyle.Tool {
	seen := map[string]struct{}{}
	for _, item := range base {
		if item.Function != nil {
			seen[item.Function.Name] = struct{}{}
		}
	}
	for _, item := range extra {
		if item.Function == nil || strings.TrimSpace(item.Function.Name) == "" {
			continue
		}
		if _, ok := seen[item.Function.Name]; ok {
			continue
		}
		base = append(base, item)
		seen[item.Function.Name] = struct{}{}
	}
	return base
}

func MarshalCatalogForDebug() string {
	payload := map[string]interface{}{
		"mcp":    mcpbridge.GetMCPSimpleInfos(),
		"skills": openclawskill.Scan(),
	}
	data, _ := json.MarshalIndent(payload, "", "  ")
	return string(data)
}

func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
	if max > 0 && len(s) > max {
		return strings.TrimSpace(s[:max]) + "..."
	}
	return s
}

func xmlEscape(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(s)
}

func DefaultToolSource(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return utils.ToolSourceMixed
	}
	return source
}
