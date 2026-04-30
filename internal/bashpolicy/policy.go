package bashpolicy

import (
	"fmt"
	"strings"
)

func validateStructural(cmdLower string) error {
	if strings.ContainsAny(cmdLower, "|;`$()<>") {
		return fmt.Errorf("含不被允许的 Shell 字符（管线、替换、重定向等）")
	}
	if strings.Contains(cmdLower, "&") {
		return fmt.Errorf("含不被允许的 & 写法（仅用 && 串联时允许拆开检查）")
	}
	return nil
}

func splitSafeCommandSegments(command string) ([]string, error) {
	parts := strings.Split(command, "&&")
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		seg := strings.TrimSpace(part)
		if seg == "" {
			return nil, fmt.Errorf("含空的串联片段")
		}
		if strings.Contains(seg, "||") || strings.Contains(seg, ";") {
			return nil, fmt.Errorf("不支持 || 或与分号串联多段脚本")
		}
		segments = append(segments, seg)
	}
	return segments, nil
}

// ValidateCommand：结构限制固定；黑名单仅来自 runtime SetRules。
func ValidateCommand(command string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return fmt.Errorf("command is empty")
	}

	segments, err := splitSafeCommandSegments(command)
	if err != nil {
		return err
	}
	for _, segment := range segments {
		cmdLower := strings.ToLower(strings.TrimSpace(segment))
		if cmdLower == "" {
			return fmt.Errorf("command segment is empty")
		}
		if err := validateRuleMatches(cmdLower, strings.TrimSpace(segment)); err != nil {
			return err
		}
		if err := validateStructural(cmdLower); err != nil {
			return err
		}
	}
	return nil
}
