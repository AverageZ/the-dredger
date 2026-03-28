package ui

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/alexzajac/the-dredger/internal/config"
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
	cfg    config.Config
	logger *slog.Logger
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

	// Collections overlay (saved list view)
	collections      []db.TagCount
	showCollections  bool
	collectionCursor int
	collectionScroll int
	activeCollection string
}

func NewApp(database *sql.DB, cfg config.Config, logger *slog.Logger) App {
	delegate := list.NewDefaultDelegate()
	l := list.New([]list.Item{}, delegate, 0, 0)
	l.Title = "The Dredger — Pending"
	l.Styles.Title = titleStyle
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowFilter(false)
	l.SetFilteringEnabled(false)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(activeColor)

	p := progress.New(progress.WithDefaultBlend())

	return App{
		db:       database,
		cfg:      cfg,
		logger:   logger,
		list:     l,
		spinner:  s,
		progress: p,
		listView: viewPending,
	}
}

func (a *App) recalcListHeight() {
	if a.height == 0 {
		return
	}
	h := a.height - 5 // base: title + margins + status bar
	if a.dredging {
		h--
	}
	if a.list.FilterState() == list.Filtering || a.list.FilterState() == list.FilterApplied {
		h--
	}
	a.list.SetSize(a.width-4, h)
}

func (a App) Init() tea.Cmd {
	return a.loadLinks
}

func (a App) loadCollections() tea.Msg {
	tags, err := db.GetTagCounts(a.db)
	return TagCountsLoadedMsg{Tags: tags, Err: err}
}

func (a App) loadCollectionLinks() tea.Msg {
	links, err := db.GetLinksByStatus(a.db, model.Saved)
	if err != nil {
		return LinksLoadedMsg{Err: err}
	}
	var filtered []model.Link
	for _, l := range links {
		for _, t := range l.Tags {
			if t == a.activeCollection {
				filtered = append(filtered, l)
				break
			}
		}
	}
	return LinksLoadedMsg{Links: filtered}
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

		svc := dredge.NewService(a.db, a.cfg.Workers, a.cfg.OllamaURL, a.cfg.OllamaModel, a.logger)
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
		if err := db.UpdateDredgeState(a.db, linkID, model.DredgeCrawling, ""); err != nil {
			a.logger.Error("failed to set crawling state", "link_id", linkID, "err", err)
		}

		svc := dredge.NewService(a.db, 1, a.cfg.OllamaURL, a.cfg.OllamaModel, a.logger)
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
		a.recalcListHeight()
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
		// Handle collections overlay input
		if a.showCollections {
			switch msg.String() {
			case keyEsc:
				a.showCollections = false
				a.collections = nil
			case "j", "down":
				if a.collectionCursor < len(a.collections)-1 {
					a.collectionCursor++
					visibleRows := a.height - 10
					if visibleRows < 4 {
						visibleRows = 4
					}
					if a.collectionCursor >= a.collectionScroll+visibleRows {
						a.collectionScroll = a.collectionCursor - visibleRows + 1
					}
				}
			case "k", "up":
				if a.collectionCursor > 0 {
					a.collectionCursor--
					if a.collectionCursor < a.collectionScroll {
						a.collectionScroll = a.collectionCursor
					}
				}
			case keyEnter:
				if a.collectionCursor < len(a.collections) {
					a.activeCollection = a.collections[a.collectionCursor].Tag
					a.showCollections = false
					a.collections = nil
					a.list.Title = fmt.Sprintf("The Dredger — %s", a.activeCollection)
					return a, a.loadCollectionLinks
				}
			case "q", keyCtrlC:
				if a.dredgeCancel != nil {
					a.dredgeCancel()
				}
				return a, tea.Quit
			}
			return a, nil
		}

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
			a.focus = NewFocusModel(a.db, a.logger, a.width, a.height, ctx, startLink)
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
		case "c":
			if a.listView == viewSaved {
				return a, a.loadCollections
			}
		case "g":
			if a.listView == viewSaved {
				a.mode = modeGrid
				a.grid = NewGridModel(a.db, a.width, a.height)
				return a, a.grid.Init()
			}
		case keyEsc:
			if a.activeCollection != "" {
				a.activeCollection = ""
				a.list.Title = "The Dredger — Saved"
				return a, a.loadSavedLinks
			}
		case "r":
			if !a.dredging {
				return a, a.startDredge()
			}
		case "/":
			a.list.SetFilteringEnabled(true)
			a.list.SetShowFilter(false)
			a.recalcListHeight()
		}

	case TagCountsLoadedMsg:
		if msg.Err == nil && len(msg.Tags) > 0 {
			a.collections = msg.Tags
			a.showCollections = true
			a.collectionCursor = 0
			a.collectionScroll = 0
		}
		return a, nil

	case LinksLoadedMsg:
		if msg.Err != nil {
			a.logger.Error("failed to load links", "err", msg.Err)
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
		a.recalcListHeight()
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
		a.recalcListHeight()
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

	prevFilterState := a.list.FilterState()
	var cmd tea.Cmd
	a.list, cmd = a.list.Update(msg)
	if a.list.FilterState() != prevFilterState {
		a.recalcListHeight()
	}
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
				a.spinner.View() + fmt.Sprintf(" Dredging... %d/%d  ", a.dredgeDone, a.dredgeTotal) + bar,
			)
		}

		viewLabel := "pending"
		if a.listView == viewSaved {
			viewLabel = "saved"
		}

		savedHints := ""
		if a.listView == viewSaved {
			savedHints = statusTextStyle.Render("c") + " collections  " +
				statusTextStyle.Render("g") + " grid  "
		}

		escHint := ""
		if a.activeCollection != "" {
			escHint = statusTextStyle.Render("esc") + " all saved  "
		}

		statusBar := statusBarStyle.Width(a.width).Render(
			statusTextStyle.Render("q") + " quit  " +
				statusTextStyle.Render("f") + " focus  " +
				statusTextStyle.Render("b") + " " + viewLabel + "  " +
				savedHints +
				escHint +
				statusTextStyle.Render("r") + " dredge  " +
				statusTextStyle.Render("/") + " filter  " +
				statusTextStyle.Render("↑↓") + " navigate",
		)

		listContent := docStyle.Render(a.list.View())

		// Build bottom chrome
		var bottomParts []string

		// Filter bar (reuse gridSearchStyle for consistency)
		if a.list.FilterState() == list.Filtering {
			filterBar := gridSearchStyle.Width(a.width).Render("/ " + a.list.FilterValue() + "█")
			bottomParts = append(bottomParts, filterBar)
		} else if a.list.FilterState() == list.FilterApplied {
			filterBar := gridSearchStyle.Width(a.width).Render(
				fmt.Sprintf("Filter: \"%s\" (%d results)", a.list.FilterValue(), len(a.list.VisibleItems())),
			)
			bottomParts = append(bottomParts, filterBar)
		}

		if enrichmentBar != "" {
			bottomParts = append(bottomParts, enrichmentBar)
		}
		bottomParts = append(bottomParts, statusBar)
		bottomChrome := lipgloss.JoinVertical(lipgloss.Left, bottomParts...)

		// Pin bottom chrome: allocate remaining height to list area
		bottomH := lipgloss.Height(bottomChrome)
		availH := a.height - bottomH
		if availH < 0 {
			availH = 0
		}
		listContent = strings.TrimRight(listContent, "\n")
		lines := strings.Split(listContent, "\n")
		if len(lines) > availH {
			lines = lines[:availH]
		}
		filler := strings.Repeat(" ", a.width)
		for len(lines) < availH {
			lines = append(lines, filler)
		}
		content = strings.Join(lines, "\n") + "\n" + bottomChrome

		// Collections overlay
		if a.showCollections {
			content = a.viewListCollections()
		}
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (a App) viewListCollections() string {
	titleLine := lipgloss.NewStyle().Bold(true).Foreground(activeColor).Render("◆ Collections")

	var rows []string
	visibleRows := a.height - 10
	if visibleRows < 4 {
		visibleRows = 4
	}

	endIdx := a.collectionScroll + visibleRows
	if endIdx > len(a.collections) {
		endIdx = len(a.collections)
	}

	for i := a.collectionScroll; i < endIdx; i++ {
		tc := a.collections[i]
		dot := lipgloss.NewStyle().Foreground(tagColorForString(tc.Tag)).Render("●")
		name := tc.Tag
		count := fmt.Sprintf("(%d)", tc.Count)

		line := fmt.Sprintf("  %s %s %s", dot, name, lipgloss.NewStyle().Foreground(lipgloss.Color("#9B9B9B")).Render(count))
		if i == a.collectionCursor {
			line = lipgloss.NewStyle().Background(lipgloss.Color("#4A3D6B")).Render(line)
		}
		rows = append(rows, line)
	}

	var scrollHints []string
	if a.collectionScroll > 0 {
		scrollHints = append(scrollHints, "↑")
	}
	if endIdx < len(a.collections) {
		scrollHints = append(scrollHints, "↓")
	}

	footerParts := []string{"j/k navigate · Enter select · Esc dismiss"}
	if len(scrollHints) > 0 {
		footerParts = append(footerParts, strings.Join(scrollHints, " "))
	}
	footer := lipgloss.NewStyle().Foreground(lipgloss.Color("#9B9B9B")).Render(strings.Join(footerParts, "  "))

	content := titleLine + "\n\n" + strings.Join(rows, "\n") + "\n\n" + footer
	overlay := serendipityOverlayStyle.Render(content)

	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, overlay)
}
