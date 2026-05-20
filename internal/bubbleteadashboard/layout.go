package bubbleteadashboard

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

const defaultTerminalWidth = 100
const minimumTerminalWidth = 24
const twoPaneWidthThreshold = 110
const paneSeparator = "  |  "
const detailLabelWidth = 19
const statusSeparator = " | "

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

func (model Model) withContentWidth(width int) Model {
	model.width = width
	return model
}

func (model Model) usesTwoPaneLayout() bool {
	return model.contentWidth() >= twoPaneWidthThreshold
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

type statusSegment struct {
	text     string
	compact  string
	priority int
}

func statusLine(width int, segments ...statusSegment) string {
	if width <= 0 {
		return ""
	}
	active := append([]statusSegment{}, segments...)
	for index := range active {
		active[index].text = strings.TrimSpace(active[index].text)
		active[index].compact = strings.TrimSpace(active[index].compact)
	}
	if line := joinStatusSegments(active, false); runeCount(line) <= width {
		return line
	}
	active, line, ok := dropOptionalStatusSegments(active, width, false)
	if ok {
		return line
	}
	if line := joinStatusSegments(active, true); runeCount(line) <= width {
		return line
	}
	if line, ok := dropStatusSegmentsToFit(active, width, true); ok {
		return line
	}
	return truncateValue(joinStatusSegments(active, true), width)
}

func dropOptionalStatusSegments(segments []statusSegment, width int, compact bool) ([]statusSegment, string, bool) {
	active := append([]statusSegment{}, segments...)
	for len(active) > 1 {
		dropIndex := lowestPriorityIndex(active)
		if active[dropIndex].priority <= 1 {
			return active, "", false
		}
		active = append(active[:dropIndex], active[dropIndex+1:]...)
		if line := joinStatusSegments(active, compact); runeCount(line) <= width {
			return active, line, true
		}
	}
	return active, "", false
}

func dropStatusSegmentsToFit(segments []statusSegment, width int, compact bool) (string, bool) {
	active := append([]statusSegment{}, segments...)
	for len(active) > 1 {
		dropIndex := lowestPriorityIndex(active)
		active = append(active[:dropIndex], active[dropIndex+1:]...)
		if line := joinStatusSegments(active, compact); runeCount(line) <= width {
			return line, true
		}
	}
	return "", false
}

func joinStatusSegments(segments []statusSegment, compact bool) string {
	parts := make([]string, 0, len(segments))
	for _, segment := range segments {
		text := segment.text
		if compact && segment.compact != "" {
			text = segment.compact
		}
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, statusSeparator)
}

func lowestPriorityIndex(segments []statusSegment) int {
	index := 0
	for candidate := 1; candidate < len(segments); candidate++ {
		if segments[candidate].priority > segments[index].priority {
			index = candidate
		}
	}
	return index
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

func lipglossWidth(value string) int {
	return lipgloss.Width(value)
}

func padRight(value string, width int) string {
	value = truncateValue(value, width)
	padding := width - lipglossWidth(value)
	if padding <= 0 {
		return value
	}
	return value + strings.Repeat(" ", padding)
}

func (model Model) detailText(output io.Writer, label string, value string) {
	value = model.truncateDetailValue(label, value)
	fmt.Fprintln(output, detailLineWithWidth(label, value, detailLabelWidth))
}

func (model Model) detailPath(output io.Writer, label string, value string) {
	fmt.Fprintln(output, detailLineWithWidth(label, model.renderInlinePath(value, detailPrefixWidth(label)), detailLabelWidth))
}

func (model Model) renderInlinePath(value string, prefixWidth int) string {
	return truncatePath(value, model.contentWidth()-prefixWidth)
}

func (model Model) truncateDetailValue(label string, value string) string {
	return truncateLogPathTokens(value, model.contentWidth()-detailPrefixWidth(label))
}

func detailPrefixWidth(label string) int {
	width := detailLabelWidth
	if len(label) > width {
		width = len(label)
	}
	return len("  ") + width + len(": ")
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
		fmt.Fprintln(output, detailLineWithWidth(label, "(none)", detailLabelWidth))
		return
	}
	fmt.Fprintln(output, detailLineWithWidth(label, "", detailLabelWidth))
	for _, value := range values {
		fmt.Fprintf(output, "    - %s\n", truncateLogPathTokens(value, model.contentWidth()-len("    - ")))
	}
}
