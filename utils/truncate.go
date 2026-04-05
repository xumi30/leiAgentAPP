package utils

// ChatTitleMaxRunes 对话标题展示与兜底截断所用的最大字符数（按 Unicode 码点计，中文一字一算）。
const ChatTitleMaxRunes = 15

// TruncateRunes returns s truncated to at most max runes (UTF-8 code points).
// If max <= 0, returns an empty string.
func TruncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
