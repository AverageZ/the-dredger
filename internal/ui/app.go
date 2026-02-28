package ui

import (
	"database/sql"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/alexzajac/the-dredger/internal/model"
)

var docStyle = lipgloss.NewStyle().Margin(1, 2)

type App struct {
	db     *sql.DB
	list   list.Model
	width  int
	height int
}

func NewApp(db *sql.DB) App {
	samples := []model.Link{
		{Title: "Hacker News", URL: "https://news.ycombinator.com", Status: model.Unprocessed},
		{Title: "Lobsters", URL: "https://lobste.rs", Status: model.Saved},
		{Title: "Go Blog", URL: "https://go.dev/blog", Status: model.Unprocessed},
		{Title: "Charm CLI Tools", URL: "https://charm.sh", Status: model.Saved},
	}

	items := make([]list.Item, len(samples))
	for i, l := range samples {
		items[i] = linkItem{link: l}
	}

	delegate := list.NewDefaultDelegate()
	l := list.New(items, delegate, 0, 0)
	l.Title = "The Dredger"
	l.Styles.Title = titleStyle

	return App{
		db:   db,
		list: l,
	}
}

func (a App) Init() tea.Cmd {
	return nil
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.list.SetSize(msg.Width-4, msg.Height-4)
		return a, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return a, tea.Quit
		}
	}

	var cmd tea.Cmd
	a.list, cmd = a.list.Update(msg)
	return a, cmd
}

func (a App) View() tea.View {
	statusBar := statusBarStyle.Width(a.width).Render(
		statusTextStyle.Render("q") + " quit  " +
			statusTextStyle.Render("/") + " filter  " +
			statusTextStyle.Render("↑↓") + " navigate",
	)

	content := docStyle.Render(a.list.View()) + "\n" + statusBar

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}
