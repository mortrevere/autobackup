package autobackup

import "strings"

const (
	maxLogPrefixLen   = 48
	logPrefixOverhead = len("[]:")
	logPathHeadLen    = 16
)

func LogPrefix(path string) string {
	labelWidth := maxLogPrefixLen - logPrefixOverhead
	return "[" + rightPadRunes(ShortPathLabel(path, labelWidth), labelWidth) + "]:"
}

func ShortPathLabel(path string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if runeLen(path) <= maxLen {
		return path
	}

	ellipsis := "..."
	if maxLen <= len(ellipsis) {
		return takeLastRunes(path, maxLen)
	}

	headLen := logPathHeadLen
	if headLen > maxLen-len(ellipsis) {
		headLen = maxLen - len(ellipsis)
	}
	tailLen := maxLen - headLen - len(ellipsis)
	tail := pathTail(path)
	if runeLen(tail) > tailLen {
		tail = takeLastRunes(tail, tailLen)
	} else {
		headLen += tailLen - runeLen(tail)
	}
	return takeFirstRunes(path, headLen) + ellipsis + tail
}

func pathTail(path string) string {
	sep := preferredSeparator(path)
	parts := splitPathComponents(path)
	if len(parts) == 0 {
		return path
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return parts[len(parts)-2] + sep + parts[len(parts)-1]
}

func preferredSeparator(path string) string {
	if strings.Contains(path, "\\") {
		return "\\"
	}
	return "/"
}

func splitPathComponents(path string) []string {
	return strings.FieldsFunc(path, func(r rune) bool {
		return r == '/' || r == '\\'
	})
}

func runeLen(s string) int {
	return len([]rune(s))
}

func takeFirstRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func takeLastRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[len(r)-n:])
}

func rightPadRunes(s string, width int) string {
	n := width - runeLen(s)
	if n <= 0 {
		return s
	}
	return s + strings.Repeat(" ", n)
}
