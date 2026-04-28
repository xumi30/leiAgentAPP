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
	path, err := localMemoryFilePath(cid)
	if err != nil {
		return err
	}
	// If there is nothing to persist, remove stale empty snapshot to reduce noise.
	if len(out.Messages) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	data, err := yaml.Marshal(&out)
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

// CleanupEmptyLocalMemoryYAML removes localmemory/{chatID}.yaml when it is empty
// (file is empty or YAML contains messages: []).
func CleanupEmptyLocalMemoryYAML(chatID string) error {
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
			return nil
		}
		return err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		_ = os.Remove(path)
		return nil
	}
	var root localMemoryYAMLRoot
	if err := yaml.Unmarshal(data, &root); err != nil {
		return err
	}
	if len(root.Messages) == 0 {
		_ = os.Remove(path)
	}
	return nil
}

// CleanupEmptyLocalMemoryYAMLDir scans localmemory/*.yaml and removes files that are empty
// or contain `messages: []`. This is best-effort housekeeping and should never fail the main flow.
func CleanupEmptyLocalMemoryYAMLDir() error {
	dir, err := LocalMemoryDir()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, ent := range entries {
		if ent == nil || ent.IsDir() {
			continue
		}
		name := ent.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".yaml") {
			continue
		}
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if len(strings.TrimSpace(string(data))) == 0 {
			_ = os.Remove(path)
			continue
		}
		var root localMemoryYAMLRoot
		if err := yaml.Unmarshal(data, &root); err != nil {
			continue
		}
		if len(root.Messages) == 0 {
			_ = os.Remove(path)
		}
	}
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
	if len(root.Messages) == 0 {
		// Best-effort cleanup for empty snapshots.
		_ = os.Remove(path)
		GetLocalMemory().Clear(cid)
		return nil
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
