package proxy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"leiAgent/internal/appruntime"
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
	// 对可执行文件更友好：默认写入 runtime root（开发时为仓库根；安装后为用户配置目录）。
	return appruntime.ResolvePath(filepath.Join("config", "config.yaml"))
}

// ReadLLMConfigForUI 读取用于编辑器展示的内容。若尚无配置文件，则返回示例内容与建议保存路径，usingExample 为 true。
func ReadLLMConfigForUI() (content string, savePath string, usingExample bool, err error) {
	savePath = DefaultConfigWritePath()
	if abs, e := filepath.Abs(savePath); e == nil {
		savePath = abs
	}
	p, ok := resolveConfigPath()
	if !ok {
		examplePathCandidates := []string{
			// 1) runtime root（开发：仓库根；安装：用户配置目录）
			appruntime.ResolvePath(filepath.Join("config", "config.example.yaml")),
			// 2) 当前工作目录（兼容旧逻辑/CLI 运行）
			filepath.Clean(filepath.Join("config", "config.example.yaml")),
		}
		// 3) 可执行文件同目录（打包发布常见布局：<exeDir>/config/config.example.yaml）
		if exe, err := os.Executable(); err == nil {
			examplePathCandidates = append(examplePathCandidates, filepath.Join(filepath.Dir(exe), "config", "config.example.yaml"))
		}

		var lastErr error
		for _, c := range examplePathCandidates {
			example, e := os.ReadFile(c)
			if e == nil {
				return string(example), savePath, true, nil
			}
			lastErr = e
		}
		return "", savePath, true, fmt.Errorf("未找到 config/config.yaml，且无法读取 config/config.example.yaml：%w", lastErr)
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
	return writeLLMConfigBytes(data)
}

func writeLLMConfigBytes(data []byte) (savedPath string, err error) {
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
