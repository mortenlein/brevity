package bubbleteadashboard

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

const defaultTerminalWidth = 100
const minimumTerminalWidth = 24

func (model Model) contentWidth() int {
	if model.width <= 0 {
		return defaultTerminalWidth
	}
	if model.width < minimumTerminalWidth {
		return minimumTerminalWidth
	}
	return model.width
}

func (model Model) renderLine(value string) string {
	return truncateValue(value, model.contentWidth())
}

func truncateValue(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if width <= 3 {
		return strings.Repeat(".", width)
	}
	if runeCount(value) <= width {
		return value
	}
	return string([]rune(value)[:width-3]) + "..."
}

func truncatePath(value string, width int) string {
	value = strings.TrimSpace(value)
	if width <= 0 {
		return ""
	}
	if runeCount(value) <= width {
		return value
	}
	if width <= 3 {
		return strings.Repeat(".", width)
	}
	suffixWidth := width - 3
	runes := []rune(value)
	if suffixWidth > len(runes) {
		suffixWidth = len(runes)
	}
	return "..." + string(runes[len(runes)-suffixWidth:])
}

func runeCount(value string) int {
	return utf8.RuneCountInString(value)
}

func (model Model) detailText(output io.Writer, label string, value string) {
	value = model.truncateDetailValue(label, value)
	fmt.Fprintln(output, detailLine(label, value))
}

func (model Model) detailPath(output io.Writer, label string, value string) {
	fmt.Fprintln(output, detailLine(label, model.renderInlinePath(value, detailPrefixWidth(label))))
}

func (model Model) renderInlinePath(value string, prefixWidth int) string {
	return truncatePath(value, model.contentWidth()-prefixWidth)
}

func (model Model) truncateDetailValue(label string, value string) string {
	return truncateLogPathTokens(value, model.contentWidth()-detailPrefixWidth(label))
}

func detailPrefixWidth(label string) int {
	return len("  ") + len(label) + len(": ")
}

func truncateLogPathTokens(value string, width int) string {
	if width <= 0 {
		return ""
	}
	parts := strings.Fields(value)
	if len(parts) == 0 {
		return truncateValue(value, width)
	}
	changed := false
	for index, part := range parts {
		if strings.HasPrefix(part, "log=") {
			parts[index] = "log=" + truncatePath(strings.TrimPrefix(part, "log="), width-len("log="))
			changed = true
		}
	}
	if changed {
		value = strings.Join(parts, " ")
	}
	return truncateValue(value, width)
}

func (model Model) renderStringList(output io.Writer, label string, values []string) {
	if len(values) == 0 {
		fmt.Fprintf(output, "  %s: (none)\n", label)
		return
	}
	fmt.Fprintf(output, "  %s:\n", label)
	for _, value := range values {
		fmt.Fprintf(output, "    - %s\n", truncateLogPathTokens(value, model.contentWidth()-len("    - ")))
	}
}
