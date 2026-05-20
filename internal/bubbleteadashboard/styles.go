package bubbleteadashboard

import "github.com/charmbracelet/lipgloss"

var dashboardStyles = struct {
	title       lipgloss.Style
	headerMeta  lipgloss.Style
	rule        lipgloss.Style
	section     lipgloss.Style
	paneTitle   lipgloss.Style
	selectedRow lipgloss.Style
	badge       lipgloss.Style
	success     lipgloss.Style
	warning     lipgloss.Style
	error       lipgloss.Style
	muted       lipgloss.Style
	footer      lipgloss.Style
	detailLabel lipgloss.Style
	detailValue lipgloss.Style
	help        lipgloss.Style
}{
	title: lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")),
	headerMeta: lipgloss.NewStyle().
		Foreground(lipgloss.Color("7")),
	rule: lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")),
	section: lipgloss.NewStyle().
		Foreground(lipgloss.Color("7")),
	paneTitle: lipgloss.NewStyle().
		Foreground(lipgloss.Color("7")),
	selectedRow: lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("236")),
	badge: lipgloss.NewStyle().
		Foreground(lipgloss.Color("7")),
	success: lipgloss.NewStyle().
		Foreground(lipgloss.Color("10")),
	warning: lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("11")),
	error: lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("9")),
	muted: lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")),
	footer: lipgloss.NewStyle().
		Foreground(lipgloss.Color("7")),
	detailLabel: lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")),
	detailValue: lipgloss.NewStyle(),
	help: lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")),
}

func sectionTitle(title string) string {
	return dashboardStyles.section.Render(title)
}

func paneTitle(title string) string {
	return dashboardStyles.paneTitle.Render(title)
}

func warningMarker() string {
	return dashboardStyles.warning.Render("!")
}

func statusBadge(text string, severity string) string {
	style := dashboardStyles.badge
	switch severity {
	case "success":
		style = style.Foreground(lipgloss.Color("10"))
	case "warning":
		style = style.Foreground(lipgloss.Color("11"))
	case "error":
		style = style.Foreground(lipgloss.Color("9"))
	case "accent":
		style = style.Foreground(lipgloss.Color("12"))
	}
	return style.Render(text)
}

func detailLine(label string, value string) string {
	return detailLineWithWidth(label, value, len(label))
}

func detailLineWithWidth(label string, value string, width int) string {
	if width < len(label) {
		width = len(label)
	}
	return "  " + dashboardStyles.detailLabel.Render(padRight(label+":", width+1)) + " " + dashboardStyles.detailValue.Render(value)
}
