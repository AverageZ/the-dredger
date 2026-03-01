package ui

import "charm.land/lipgloss/v2"

var (
	activeColor = lipgloss.Color("#7D56F4")
	accentColor = lipgloss.Color("#04B575")

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

	enrichmentBarStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFDF5")).
				Background(lipgloss.Color("#4A3D6B")).
				Padding(0, 1)

	// Focus mode colors
	keepColor   = lipgloss.Color("#04B575")
	pruneColor  = lipgloss.Color("#FF4040")
	snoozeColor = lipgloss.Color("#FFB347")

	// Focus mode card styles
	cardBorderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(activeColor).
			Padding(1, 2)

	domainHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FFFDF5")).
				Background(activeColor).
				Padding(0, 1)

	cardTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFDF5"))

	cardURLStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9B9B9B")).
			Italic(true)

	cardDescStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#C0C0C0"))

	tagPillStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFDF5")).
			Background(activeColor).
			Padding(0, 1)

	undoToastStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFDF5")).
			Background(lipgloss.Color("#4A3D6B")).
			Italic(true).
			Padding(0, 1)

	completionStyle = lipgloss.NewStyle().
			Foreground(keepColor).
			Bold(true).
			Align(lipgloss.Center)
)
