package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v2"

	"leiAgent/logging"
)

// localMemoryYAMLRoot 与 GetLocalMemoryMessages 展示结构一致，便于人工查看与版本控制。
type localMemoryYAMLRoot struct {
	Messages []localMemoryYAMLMsg `yaml:"messages"`
}

type localMemoryYAMLMsg struct {
	Role       MessageRole `yaml:"role"`
	Content    string      `yaml:"content,omitempty"`
	ToolCallID string      `yaml:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall  `yaml:"tool_calls,omitempty"`
}

// LocalMemoryDir 返回工作目录下 localmemory 目录的绝对路径（不存在则创建）。
func LocalMemoryDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cwd, "localmemory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func localMemoryFilePath(chatID string) (string, error) {
	cid := strings.TrimSpace(chatID)
	if cid == "" {
		return "", fmt.Errorf("chatID 为空")
	}
	dir, err := LocalMemoryDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, cid+".yaml"), nil
}

// PersistLocalMemoryToYAMLFile 将当前 chat 在内存中的 localMemory 写入 localmemory/{chatID}.yaml（与「本地记忆」弹窗同源数据；跳过 nil 槽位）。
func PersistLocalMemoryToYAMLFile(chatID string) error {
	cid := strings.TrimSpace(chatID)
	if cid == "" {
		return nil
	}
	msgs := GetLocalMemory().GetMessages(cid)
	out := localMemoryYAMLRoot{
		Messages: make([]localMemoryYAMLMsg, 0, len(msgs)),
	}
	for _, m := range msgs {
		if m == nil {
			continue
		}
		out.Messages = append(out.Messages, localMemoryYAMLMsg{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
			ToolCalls:  m.ToolCalls,
		})
	}
	data, err := yaml.Marshal(&out)
	if err != nil {
		return err
	}
	path, err := localMemoryFilePath(cid)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	logging.Info("已写入本地记忆快照: %s", path)
	return nil
}

// LoadLocalMemoryFromYAMLFile 从 localmemory/{chatID}.yaml 按顺序恢复消息（先 Clear 再逐条 AddMessage）。文件不存在则清空该 chat 的内存。
func LoadLocalMemoryFromYAMLFile(chatID string) error {
	cid := strings.TrimSpace(chatID)
	if cid == "" {
		return nil
	}
	path, err := localMemoryFilePath(cid)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			GetLocalMemory().Clear(cid)
			return nil
		}
		return err
	}
	var root localMemoryYAMLRoot
	if err := yaml.Unmarshal(data, &root); err != nil {
		return err
	}
	lm := GetLocalMemory()
	lm.Clear(cid)
	for _, row := range root.Messages {
		msg := &Message{
			Role:       row.Role,
			Content:    row.Content,
			ToolCallID: row.ToolCallID,
			ToolCalls:  row.ToolCalls,
		}
		lm.AddMessage(cid, msg)
	}
	logging.Info("已从 YAML 恢复本地记忆: %s (%d 条)", path, len(root.Messages))
	if len(root.Messages) > AutoCompressYAMLMessageThreshold {
		logging.Info("YAML 消息条数 %d 超过阈值 %d，触发记忆压缩 chatID=%s", len(root.Messages), AutoCompressYAMLMessageThreshold, cid)
		invokeAutoCompressHook(cid)
	}
	return nil
}
