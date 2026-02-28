package ui

import "charm.land/lipgloss/v2"

var (
	activeColor   = lipgloss.Color("#7D56F4")
	inactiveColor = lipgloss.Color("#626262")
	accentColor   = lipgloss.Color("#04B575")

	activeBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(activeColor).
				Padding(0, 1)

	inactiveBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(inactiveColor).
				Padding(0, 1)

	titleStyle = lipgloss.NewStyle().
			Foreground(activeColor).
			Bold(true).
			Padding(0, 1)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFDF5")).
			Background(lipgloss.Color("#353533")).
			Padding(0, 1)

	statusTextStyle = lipgloss.NewStyle().
			Foreground(accentColor)

	itemTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFDF5")).
			Bold(true)

	itemDescStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9B9B9B"))
)
