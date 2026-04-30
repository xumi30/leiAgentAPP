package bashpolicy

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// Rule 表单项配置（YAML / JSON / UI 同源）。
type Rule struct {
	Command     string `yaml:"command" json:"command"`
	Description string `yaml:"description" json:"description"`
	Severity    string `yaml:"severity" json:"severity"` // high | medium | low | pending（待定）
	Enabled     bool   `yaml:"enabled" json:"enabled"`
	MatchKind   string `yaml:"match_kind,omitempty" json:"matchKind"` // substring | regex
}

var (
	ruleMu      sync.RWMutex
	activeRules []compiledRule
)

type compiledRule struct {
	commandSubstr string // lower-case substring to match (substring kind)
	description   string // for errors
	regex         *regexp.Regexp
	isRegex       bool
	severity      string // unused in validation but kept for completeness
}

// DefaultRules 与历史内置黑名单一致的可编辑默认值。
func DefaultRules() []Rule {
	return []Rule{
		{Command: "rm -rf /", Description: "根分区破坏性删除片段", Severity: "high", Enabled: true, MatchKind: "substring"},
		{Command: ":(){:|:&};:", Description: "进程炸弹（资源耗尽）", Severity: "high", Enabled: true, MatchKind: "substring"},
		{Command: "mkfs", Description: "磁盘格式化 / 建文件系统", Severity: "high", Enabled: true, MatchKind: "substring"},
		{Command: "dd if=/dev/zero", Description: "对盘填零一类的破坏性行为", Severity: "high", Enabled: true, MatchKind: "substring"},
		{Command: `(?i)^dd\s+.*of=/dev/(sd|nvme|vd|mmcblk)`, Description: "dd 输出到整盘块设备（of=/dev/sd* 等）", Severity: "high", Enabled: true, MatchKind: "regex"},
		{Command: "chmod -r 777 /", Description: "对根目录递归 chmod 777（权限失控）", Severity: "high", Enabled: true, MatchKind: "substring"},
		{Command: "chmod 777 /", Description: "根目录 chmod 777（即使非递归也极危）", Severity: "high", Enabled: true, MatchKind: "substring"},
		{Command: "chown -r root:root /", Description: "递归将根目录属主改为 root（易致系统不可用）", Severity: "high", Enabled: true, MatchKind: "substring"},
		{Command: "mv /* /dev/null", Description: "将根下内容移入空设备（毁灭性）", Severity: "high", Enabled: true, MatchKind: "substring"},
		{Command: `(?i)^mount\s+.+\/\s*$`, Description: "将分区挂载到根目录覆盖系统", Severity: "high", Enabled: true, MatchKind: "regex"},
		{Command: "cp /dev/null /bin/", Description: "用空设备覆盖 /bin 下可执行文件", Severity: "high", Enabled: true, MatchKind: "substring"},
		{Command: "ln -sf /dev/null /bin/", Description: "将 /bin 下程序符号链接到空设备", Severity: "high", Enabled: true, MatchKind: "substring"},
		{Command: `(?i)iptables.*-\s*[fx](?:\s|$)`, Description: "iptables/ip6tables 清空链或删除表（易导致网络暴露）", Severity: "high", Enabled: true, MatchKind: "regex"},
		{Command: `(?i)ip6tables.*-\s*[fx](?:\s|$)`, Description: "ip6tables 清空规则链", Severity: "high", Enabled: true, MatchKind: "regex"},
		{Command: "nft flush ruleset", Description: "清空 nftables 规则集（防火墙失效）", Severity: "high", Enabled: true, MatchKind: "substring"},
		{Command: "find / -delete", Description: "自根目录起 find -delete（批量删文件）", Severity: "high", Enabled: true, MatchKind: "substring"},
		{Command: "find / -exec rm", Description: "自根目录起 find 执行 rm", Severity: "high", Enabled: true, MatchKind: "substring"},
		{Command: "wipefs -a", Description: "擦除盘上文件系统签名（易导致数据不可用）", Severity: "high", Enabled: true, MatchKind: "substring"},
		{Command: "zfs destroy", Description: "销毁 ZFS 数据集/存储池（不可逆）", Severity: "high", Enabled: true, MatchKind: "substring"},
		{Command: "shutdown", Description: "系统关机", Severity: "medium", Enabled: true, MatchKind: "substring"},
		{Command: "reboot", Description: "重启系统", Severity: "medium", Enabled: true, MatchKind: "substring"},
		{Command: "halt", Description: "停机", Severity: "medium", Enabled: true, MatchKind: "substring"},
		{Command: "poweroff", Description: "断电关机", Severity: "medium", Enabled: true, MatchKind: "substring"},
		{Command: "del /f /s /q", Description: "Windows 强制递归删除文件", Severity: "high", Enabled: true, MatchKind: "substring"},
		{Command: "rmdir /s /q", Description: "Windows 递归删除目录", Severity: "high", Enabled: true, MatchKind: "substring"},
		{Command: `(?i)^\s*format(?:\s|$)`, Description: "Windows 行首 format 格式化命令", Severity: "high", Enabled: true, MatchKind: "regex"},
	}
}

func normalizeSeverity(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "high", "h", "高":
		return "high"
	case "low", "l", "低":
		return "low"
	case "pending", "待定", "tbd":
		return "pending"
	default:
		return "medium"
	}
}

func normalizeMatchKind(s string) string {
	if strings.EqualFold(strings.TrimSpace(s), "regex") {
		return "regex"
	}
	return "substring"
}

// CompileRules 将规则编译为运行时形态；任一正则非法则报错。
func CompileRules(in []Rule) ([]compiledRule, error) {
	out := make([]compiledRule, 0, len(in))
	for _, r := range in {
		if !r.Enabled {
			continue
		}
		cmd := strings.TrimSpace(r.Command)
		if cmd == "" {
			continue
		}
		desc := strings.TrimSpace(r.Description)
		if desc == "" {
			desc = cmd
		}
		kind := normalizeMatchKind(r.MatchKind)
		switch kind {
		case "regex":
			re, err := regexp.Compile(cmd)
			if err != nil {
				return nil, fmt.Errorf("规则正则无效（%s）: %w", cmd, err)
			}
			out = append(out, compiledRule{description: desc, regex: re, isRegex: true, severity: normalizeSeverity(r.Severity)})
		default:
			out = append(out, compiledRule{commandSubstr: strings.ToLower(cmd), description: desc, isRegex: false, severity: normalizeSeverity(r.Severity)})
		}
	}
	return out, nil
}

// SetRules 替换运行时黑名单（仅 Enabled 的规则参与校验）。
func SetRules(rules []Rule) error {
	compiled, err := CompileRules(rules)
	if err != nil {
		return err
	}
	ruleMu.Lock()
	activeRules = compiled
	ruleMu.Unlock()
	return nil
}

func validateRuleMatches(cmdLower string, originalSegment string) error {
	ruleMu.RLock()
	defer ruleMu.RUnlock()
	for _, c := range activeRules {
		if c.isRegex {
			if c.regex != nil && c.regex.MatchString(strings.TrimSpace(originalSegment)) {
				return fmt.Errorf("命中黑名单：%s", c.description)
			}
			continue
		}
		if c.commandSubstr != "" && strings.Contains(cmdLower, c.commandSubstr) {
			return fmt.Errorf("命中黑名单：%s", c.description)
		}
	}
	return nil
}

// MergeLegacyExtra 将旧的 extra_blocked_substrings 转成规则追加到 base。
func MergeLegacyExtra(base []Rule, extra []string) []Rule {
	next := append([]Rule(nil), base...)
	for _, x := range extra {
		x = strings.TrimSpace(x)
		if x == "" {
			continue
		}
		next = append(next, Rule{
			Command: x, Description: "历史配置 extra_blocked_substrings", Severity: "high", Enabled: true, MatchKind: "substring",
		})
	}
	return next
}

// NormalizeInputRows 去掉空命令行；若全无有效行则退回默认。
func NormalizeInputRows(in []Rule) []Rule {
	out := make([]Rule, 0, len(in))
	for _, r := range in {
		if strings.TrimSpace(r.Command) == "" {
			continue
		}
		r.Command = strings.TrimSpace(r.Command)
		r.Description = strings.TrimSpace(r.Description)
		r.Severity = normalizeSeverity(r.Severity)
		r.MatchKind = normalizeMatchKind(r.MatchKind)
		out = append(out, r)
	}
	if len(out) == 0 {
		return DefaultRules()
	}
	return out
}

// MergeFromYAML 从 YAML 解码结果合并：优先 rules；否则 extra_blocked_substrings；都没有则默认。
func MergeFromYAML(rules []Rule, extra []string) []Rule {
	if len(rules) > 0 {
		return NormalizeInputRows(rules)
	}
	if len(extra) > 0 {
		return MergeLegacyExtra(DefaultRules(), extra)
	}
	return DefaultRules()
}
