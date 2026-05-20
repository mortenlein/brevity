package bubbleteadashboard

import "github.com/charmbracelet/lipgloss"

var dashboardStyles = struct {
	title       lipgloss.Style
	rule        lipgloss.Style
	section     lipgloss.Style
	selectedRow lipgloss.Style
	warning     lipgloss.Style
	footer      lipgloss.Style
	detailLabel lipgloss.Style
	detailValue lipgloss.Style
	help        lipgloss.Style
}{
	title: lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("4")).
		Padding(0, 1),
	rule: lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")),
	section: lipgloss.NewStyle().
		Bold(true).
		Underline(true),
	selectedRow: lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("0")).
		Background(lipgloss.Color("15")),
	warning: lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("11")),
	footer: lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")),
	detailLabel: lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("14")),
	detailValue: lipgloss.NewStyle(),
	help: lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")),
}

func sectionTitle(title string) string {
	return dashboardStyles.section.Render(title)
}

func warningMarker() string {
	return dashboardStyles.warning.Render("!")
}

func detailLine(label string, value string) string {
	return "  " + dashboardStyles.detailLabel.Render(label+":") + " " + dashboardStyles.detailValue.Render(value)
}
