package proxy

import (
	"fmt"

	"leiAgent/internal/bashpolicy"
)

// ShellSafetyFormState 终端黑名单设置表单。
type ShellSafetyFormState struct {
	Path  string           `json:"path"`
	Rules []bashpolicy.Rule `json:"rules"`
}

func GetShellSafetyFormState() (ShellSafetyFormState, error) {
	pathShown := GetResolvedConfigPath()
	if pathShown == "" {
		pathShown = DefaultConfigWritePath()
	}
	root, path, err := readConfigRoot()
	if err != nil {
		return ShellSafetyFormState{Path: pathShown, Rules: bashpolicy.DefaultRules()}, nil
	}
	if path == "" {
		return ShellSafetyFormState{Path: pathShown, Rules: bashpolicy.DefaultRules()}, nil
	}
	rules := bashpolicy.MergeFromYAML(root.ShellSafety.Rules, root.ShellSafety.ExtraBlockedSubstrings)
	return ShellSafetyFormState{Path: pathShown, Rules: rules}, nil
}

func SaveShellSafetyForm(rules []bashpolicy.Rule) (savedPath string, err error) {
	norm := bashpolicy.NormalizeInputRows(rules)
	if _, err := bashpolicy.CompileRules(norm); err != nil {
		return "", err
	}

	content, _, _, err := ReadLLMConfigForUI()
	if err != nil {
		return "", err
	}

	doc, err := parseYAMLDocumentNode(content)
	if err != nil {
		return "", fmt.Errorf("YAML 解析失败：%w", err)
	}

	payload := map[string][]bashpolicy.Rule{"rules": norm}
	ssNode, err := nodeFromValue(payload)
	if err != nil {
		return "", fmt.Errorf("序列化 shell_safety 失败：%w", err)
	}
	upsertRootKey(doc, "shell_safety", ssNode)

	out, err := marshalYAMLDocumentNode(doc)
	if err != nil {
		return "", err
	}
	wp, err := writeLLMConfigBytes(out)
	if err != nil {
		return "", err
	}
	InitShellSafetyFromConfig()
	return wp, nil
}
