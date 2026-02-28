package ui

import (
	"context"
	"database/sql"
	"fmt"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/alexzajac/the-dredger/internal/db"
	"github.com/alexzajac/the-dredger/internal/dredge"
)

var docStyle = lipgloss.NewStyle().Margin(1, 2)

type App struct {
	db     *sql.DB
	list   list.Model
	width  int
	height int

	spinner      spinner.Model
	progress     progress.Model
	dredging     bool
	dredgeTotal  int
	dredgeDone   int
	dredgeCancel context.CancelFunc
	resultsCh    <-chan dredge.Result
}

func NewApp(database *sql.DB) App {
	delegate := list.NewDefaultDelegate()
	l := list.New([]list.Item{}, delegate, 0, 0)
	l.Title = "The Dredger"
	l.Styles.Title = titleStyle

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(activeColor)

	p := progress.New(progress.WithDefaultBlend())

	return App{
		db:       database,
		list:     l,
		spinner:  s,
		progress: p,
	}
}

func (a App) Init() tea.Cmd {
	return a.loadLinks
}

func (a App) loadLinks() tea.Msg {
	links, err := db.GetLinks(a.db)
	return LinksLoadedMsg{Links: links, Err: err}
}

func (a App) startDredge() tea.Cmd {
	return func() tea.Msg {
		unprocessed, err := db.GetUnprocessedLinks(a.db)
		if err != nil {
			return DredgeDoneMsg{Err: fmt.Errorf("fetch unprocessed links: %w", err)}
		}
		if len(unprocessed) == 0 {
			return DredgeDoneMsg{}
		}

		ctx, cancel := context.WithCancel(context.Background())

		svc := dredge.NewService(a.db, 4)
		go svc.Run(ctx, unprocessed)

		return dredgeStartInternal{
			total:   len(unprocessed),
			cancel:  cancel,
			results: svc.Results(),
		}
	}
}

type dredgeStartInternal struct {
	total   int
	cancel  context.CancelFunc
	results <-chan dredge.Result
}

func waitForResult(ch <-chan dredge.Result) tea.Cmd {
	return func() tea.Msg {
		result, ok := <-ch
		if !ok {
			return DredgeDoneMsg{}
		}
		return DredgeResultMsg{Result: result}
	}
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		listHeight := msg.Height - 4
		if a.dredging {
			listHeight -= 1
		}
		a.list.SetSize(msg.Width-4, listHeight)
		return a, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			if a.dredgeCancel != nil {
				a.dredgeCancel()
			}
			return a, tea.Quit
		case "r":
			if !a.dredging {
				return a, a.startDredge()
			}
		}

	case LinksLoadedMsg:
		if msg.Err != nil {
			return a, nil
		}
		items := make([]list.Item, len(msg.Links))
		for i, l := range msg.Links {
			items[i] = linkItem{link: l}
		}
		a.list.SetItems(items)
		return a, a.startDredge()

	case dredgeStartInternal:
		a.dredging = true
		a.dredgeTotal = msg.total
		a.dredgeDone = 0
		a.dredgeCancel = msg.cancel
		a.resultsCh = msg.results
		if a.height > 0 {
			a.list.SetSize(a.width-4, a.height-5)
		}
		return a, tea.Batch(a.spinner.Tick, waitForResult(a.resultsCh))

	case DredgeResultMsg:
		a.dredgeDone++
		a.updateListItem(msg.Result)
		var cmds []tea.Cmd
		if a.dredgeTotal > 0 {
			cmds = append(cmds, a.progress.SetPercent(float64(a.dredgeDone)/float64(a.dredgeTotal)))
		}
		cmds = append(cmds, waitForResult(a.resultsCh))
		return a, tea.Batch(cmds...)

	case DredgeDoneMsg:
		a.dredging = false
		if a.height > 0 {
			a.list.SetSize(a.width-4, a.height-4)
		}
		return a, nil

	case spinner.TickMsg:
		if a.dredging {
			var cmd tea.Cmd
			a.spinner, cmd = a.spinner.Update(msg)
			return a, cmd
		}
		return a, nil

	case progress.FrameMsg:
		var cmd tea.Cmd
		a.progress, cmd = a.progress.Update(msg)
		return a, cmd
	}

	var cmd tea.Cmd
	a.list, cmd = a.list.Update(msg)
	return a, cmd
}

func (a *App) updateListItem(result dredge.Result) {
	items := a.list.Items()
	for i, item := range items {
		li, ok := item.(linkItem)
		if !ok {
			continue
		}
		if li.link.ID == result.LinkID {
			if result.Err == nil {
				li.link.Title = result.Title
				li.link.Description = result.Description
			}
			a.list.SetItem(i, li)
			return
		}
	}
}

func (a App) View() tea.View {
	var enrichmentBar string
	if a.dredging {
		bar := a.progress.ViewAs(float64(a.dredgeDone) / max(float64(a.dredgeTotal), 1))
		enrichmentBar = enrichmentBarStyle.Width(a.width).Render(
			a.spinner.View()+fmt.Sprintf(" Dredging... %d/%d  ", a.dredgeDone, a.dredgeTotal)+bar,
		) + "\n"
	}

	statusBar := statusBarStyle.Width(a.width).Render(
		statusTextStyle.Render("q") + " quit  " +
			statusTextStyle.Render("r") + " dredge  " +
			statusTextStyle.Render("/") + " filter  " +
			statusTextStyle.Render("↑↓") + " navigate",
	)

	content := docStyle.Render(a.list.View()) + "\n" + enrichmentBar + statusBar

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}
