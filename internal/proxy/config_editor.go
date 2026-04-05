package proxy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GetResolvedConfigPath 返回当前使用的配置文件绝对路径；未找到时返回空字符串。
func GetResolvedConfigPath() string {
	p, ok := resolveConfigPath()
	if !ok {
		return ""
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// DefaultConfigWritePath 保存配置时使用的路径：已有配置则覆盖同一路径，否则为 ./config/config.yaml（相对当前工作目录）。
func DefaultConfigWritePath() string {
	if p, ok := resolveConfigPath(); ok {
		return p
	}
	return filepath.Clean("config/config.yaml")
}

// ReadLLMConfigForUI 读取用于编辑器展示的内容。若尚无配置文件，则返回示例内容与建议保存路径，usingExample 为 true。
func ReadLLMConfigForUI() (content string, savePath string, usingExample bool, err error) {
	savePath = DefaultConfigWritePath()
	if abs, e := filepath.Abs(savePath); e == nil {
		savePath = abs
	}
	p, ok := resolveConfigPath()
	if !ok {
		example, e := os.ReadFile("config/config.example.yaml")
		if e != nil {
			return "", savePath, true, fmt.Errorf("未找到 config/config.yaml，且无法读取 config/config.example.yaml：%w", e)
		}
		return string(example), savePath, true, nil
	}
	if abs, e := filepath.Abs(p); e == nil {
		savePath = abs
	} else {
		savePath = p
	}
	data, e := os.ReadFile(p)
	if e != nil {
		return "", savePath, false, e
	}
	return string(data), savePath, false, nil
}

// SaveLLMConfigText 校验并写入配置文件（必要时创建 config 目录）。
func SaveLLMConfigText(content string) (savedPath string, err error) {
	data := []byte(strings.ReplaceAll(content, "\r\n", "\n"))
	if err := ValidateLLMConfigYAML(data); err != nil {
		return "", err
	}
	path := DefaultConfigWritePath()
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("创建配置目录失败：%w", err)
		}
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("写入配置失败：%w", err)
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs, nil
	}
	return path, nil
}
