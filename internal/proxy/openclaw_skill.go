package proxy

import (
	"context"
	"leiAgent/internal/openclawskill"
)

type OpenClawSkillState struct {
	WorkspaceRoot string                    `json:"workspaceRoot"`
	SkillsRoot    string                    `json:"skillsRoot"`
	Skills        []openclawskill.SkillInfo `json:"skills"`
}

func GetOpenClawSkillState() OpenClawSkillState {
	return OpenClawSkillState{
		WorkspaceRoot: openclawskill.WorkspaceRoot(),
		SkillsRoot:    openclawskill.SkillsRoot(),
		Skills:        openclawskill.Scan(),
	}
}

func InstallOpenClawSkill(ctx context.Context, input string) (openclawskill.InstallResult, error) {
	return openclawskill.Install(ctx, input)
}

func DeleteOpenClawSkill(path string) (openclawskill.OpenClawDeleteResult, error) {
	return openclawskill.Delete(path)
}

func InstallOpenClawSkillDeps(path string) (openclawskill.OpenClawDepsResult, error) {
	return openclawskill.InstallDeps(path)
}
