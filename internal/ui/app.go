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
	"github.com/alexzajac/the-dredger/internal/model"
)

const keyCtrlC = "ctrl+c"

var docStyle = lipgloss.NewStyle().Margin(1, 2)

type appMode int

const (
	modeList  appMode = 0
	modeFocus appMode = 1
	modeGrid  appMode = 2
)

type listView int

const (
	viewPending listView = iota
	viewSaved
)

type App struct {
	db     *sql.DB
	list   list.Model
	width  int
	height int

	mode     appMode
	focus    FocusModel
	grid     GridModel
	listView listView

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
	l.Title = "The Dredger — Pending"
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
		listView: viewPending,
	}
}

func (a App) Init() tea.Cmd {
	return a.loadLinks
}

func (a App) loadLinks() tea.Msg {
	links, err := db.GetLinksByStatus(a.db, model.Unprocessed)
	return LinksLoadedMsg{Links: links, Err: err}
}

func (a App) loadSavedLinks() tea.Msg {
	links, err := db.GetLinksByStatus(a.db, model.Saved)
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

func (a App) dredgeSingleLink(linkID int64, url string) tea.Cmd {
	return func() tea.Msg {
		// Set state to crawling
		_ = db.UpdateDredgeState(a.db, linkID, model.DredgeCrawling, "")

		svc := dredge.NewService(a.db, 1)
		link := model.Link{ID: linkID, URL: url}
		ctx := context.Background()

		go svc.Run(ctx, []model.Link{link})

		result := <-svc.Results()

		if result.Err != nil {
			return DredgeLinkResultMsg{
				LinkID: linkID,
				State:  model.DredgeCapsized,
				Error:  result.Err.Error(),
			}
		}

		return DredgeLinkResultMsg{
			LinkID:      linkID,
			State:       model.DredgeComplete,
			Title:       result.Title,
			Description: result.Description,
			Summary:     result.Summary,
			Tags:        result.Tags,
		}
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
		a.focus.width = msg.Width
		a.focus.height = msg.Height
		a.grid.width = msg.Width
		a.grid.height = msg.Height
		a.grid.recalcLayout()
		return a, nil

	case FocusExitMsg:
		a.mode = modeList
		if a.listView == viewSaved {
			return a, a.loadSavedLinks
		}
		return a, a.loadLinks
	}

	// Delegate to focus mode
	if a.mode == modeFocus {
		return a.updateFocus(msg)
	}

	if a.mode == modeGrid {
		return a.updateGrid(msg)
	}

	return a.updateList(msg)
}

func (a App) updateFocus(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", keyCtrlC:
			if a.dredgeCancel != nil {
				a.dredgeCancel()
			}
			return a, tea.Quit
		}

	case TriggerDredgeLinkMsg:
		cmd := a.dredgeSingleLink(msg.LinkID, msg.URL)
		// Update current link state to crawling immediately
		if a.focus.current != nil && a.focus.current.ID == msg.LinkID {
			a.focus.current.DredgeState = model.DredgeCrawling
		}
		return a, cmd
	}

	var cmd tea.Cmd
	a.focus, cmd = a.focus.Update(msg)
	return a, cmd
}

func (a App) updateGrid(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if !a.grid.searching && !a.grid.showSerendipity {
			switch msg.String() {
			case "q", keyCtrlC:
				if a.dredgeCancel != nil {
					a.dredgeCancel()
				}
				return a, tea.Quit
			}
		}

	case GridExitMsg:
		a.mode = modeList
		return a, a.loadSavedLinks
	}

	var cmd tea.Cmd
	a.grid, cmd = a.grid.Update(msg)
	return a, cmd
}

func (a App) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if a.list.FilterState() == list.Filtering {
			break // let list handle filter input
		}
		switch msg.String() {
		case "q", keyCtrlC:
			if a.dredgeCancel != nil {
				a.dredgeCancel()
			}
			return a, tea.Quit
		case "f":
			a.mode = modeFocus
			ctx := focusPending
			if a.listView == viewSaved {
				ctx = focusSaved
			}
			var startLink *model.Link
			if sel, ok := a.list.SelectedItem().(linkItem); ok {
				link := sel.link
				startLink = &link
			}
			a.focus = NewFocusModel(a.db, a.width, a.height, ctx, startLink)
			return a, a.focus.Init()
		case "b":
			if a.listView == viewPending {
				a.listView = viewSaved
				a.list.Title = "The Dredger — Saved"
				return a, a.loadSavedLinks
			}
			a.listView = viewPending
			a.list.Title = "The Dredger — Pending"
			return a, a.loadLinks
		case "g":
			if a.listView == viewSaved {
				a.mode = modeGrid
				a.grid = NewGridModel(a.db, a.width, a.height)
				return a, a.grid.Init()
			}
		case "r":
			if !a.dredging {
				return a, a.startDredge()
			}
		case "/":
			a.list.SetFilteringEnabled(true)
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
		if a.listView == viewPending && !a.dredging {
			return a, a.startDredge()
		}
		return a, nil

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
				li.link.Summary = result.Summary
				li.link.Tags = result.Tags
			}
			a.list.SetItem(i, li)
			return
		}
	}
}

func (a App) View() tea.View {
	var content string

	switch a.mode {
	case modeFocus:
		content = a.focus.View()
	case modeGrid:
		content = a.grid.View()
	default:
		var enrichmentBar string
		if a.dredging {
			bar := a.progress.ViewAs(float64(a.dredgeDone) / max(float64(a.dredgeTotal), 1))
			enrichmentBar = enrichmentBarStyle.Width(a.width).Render(
				a.spinner.View()+fmt.Sprintf(" Dredging... %d/%d  ", a.dredgeDone, a.dredgeTotal)+bar,
			) + "\n"
		}

		viewLabel := "pending"
		if a.listView == viewSaved {
			viewLabel = "saved"
		}

		gridHint := ""
		if a.listView == viewSaved {
			gridHint = statusTextStyle.Render("g") + " grid  "
		}

		statusBar := statusBarStyle.Width(a.width).Render(
			statusTextStyle.Render("q") + " quit  " +
				statusTextStyle.Render("f") + " focus  " +
				statusTextStyle.Render("b") + " " + viewLabel + "  " +
				gridHint +
				statusTextStyle.Render("r") + " dredge  " +
				statusTextStyle.Render("/") + " filter  " +
				statusTextStyle.Render("↑↓") + " navigate",
		)

		content = docStyle.Render(a.list.View()) + "\n" + enrichmentBar + statusBar
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}
