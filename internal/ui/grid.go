package ui

import (
	"database/sql"
	"fmt"
	"hash/fnv"
	"image/color"
	"net/url"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/alexzajac/the-dredger/internal/db"
	"github.com/alexzajac/the-dredger/internal/model"
	"github.com/atotto/clipboard"
)

const (
	gridCellMinW  = 28
	gridCellMaxW  = 40
	gridCellH     = 6
	gridGutter    = 2
	quickLookW    = 36
	quickLookMinW = 80
)

var (
	gridSelectedBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(activeColor)

	gridNormalBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#555555"))

	gridQuickLookStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(activeColor).
				Padding(1, 2)

	gridSearchStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFDF5")).
			Background(lipgloss.Color("#4A3D6B")).
			Padding(0, 1)

	serendipityCardStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#FFB347")).
				Padding(1, 2)

	serendipityOverlayStyle = lipgloss.NewStyle().
				Border(lipgloss.DoubleBorder()).
				BorderForeground(activeColor).
				Padding(1, 2)

	// Tag colors for deterministic cell coloring
	tagColors = []color.Color{
		lipgloss.Color("#E06C75"), lipgloss.Color("#98C379"),
		lipgloss.Color("#61AFEF"), lipgloss.Color("#C678DD"),
		lipgloss.Color("#E5C07B"), lipgloss.Color("#56B6C2"),
		lipgloss.Color("#BE5046"), lipgloss.Color("#7EC8E3"),
	}
)

type GridModel struct {
	db       *sql.DB
	links    []model.Link
	filtered []model.Link

	cols    int
	cellW   int
	cursorX int
	cursorY int
	scrollY int
	visRows int

	searching   bool
	searchQuery string

	serendipityLinks []model.Link
	showSerendipity  bool

	width  int
	height int
}

func NewGridModel(database *sql.DB, width, height int) GridModel {
	g := GridModel{
		db:     database,
		width:  width,
		height: height,
	}
	g.recalcLayout()
	return g
}

func (g GridModel) Init() tea.Cmd {
	return g.loadGridLinks
}

func (g GridModel) loadGridLinks() tea.Msg {
	links, err := db.GetLinksByStatus(g.db, model.Saved)
	return GridLinksLoadedMsg{Links: links, Err: err}
}

func (g GridModel) loadSerendipity() tea.Msg {
	links, err := db.GetRandomSavedLinks(g.db, 3)
	return SerendipityResultMsg{Links: links, Err: err}
}

func (g *GridModel) recalcLayout() {
	availW := g.width - 4 // margin
	if availW > quickLookMinW {
		availW -= quickLookW + gridGutter
	}
	if availW < gridCellMinW {
		availW = gridCellMinW
	}

	g.cols = (availW + gridGutter) / (gridCellMinW + gridGutter)
	if g.cols < 1 {
		g.cols = 1
	}

	g.cellW = (availW - (g.cols-1)*gridGutter) / g.cols
	if g.cellW > gridCellMaxW {
		g.cellW = gridCellMaxW
	}
	if g.cellW < gridCellMinW {
		g.cellW = gridCellMinW
	}

	// Reserve lines for: status bar (1), search bar (1), padding (2)
	usableH := g.height - 4
	if usableH < gridCellH {
		usableH = gridCellH
	}
	g.visRows = usableH / (gridCellH + 1) // +1 for row gap
	if g.visRows < 1 {
		g.visRows = 1
	}
}

func (g *GridModel) activeLinks() []model.Link {
	if g.searching && g.searchQuery != "" {
		return g.filtered
	}
	return g.links
}

func (g *GridModel) totalRows() int {
	n := len(g.activeLinks())
	if n == 0 {
		return 0
	}
	return (n + g.cols - 1) / g.cols
}

func (g *GridModel) clampCursor() {
	links := g.activeLinks()
	n := len(links)
	if n == 0 {
		g.cursorX, g.cursorY = 0, 0
		return
	}
	total := g.totalRows()
	if g.cursorY >= total {
		g.cursorY = total - 1
	}
	lastRowItems := n - g.cursorY*g.cols
	if lastRowItems > g.cols {
		lastRowItems = g.cols
	}
	if g.cursorX >= lastRowItems {
		g.cursorX = lastRowItems - 1
	}
	if g.cursorX < 0 {
		g.cursorX = 0
	}
	if g.cursorY < 0 {
		g.cursorY = 0
	}
}

func (g *GridModel) ensureVisible() {
	if g.cursorY < g.scrollY {
		g.scrollY = g.cursorY
	}
	if g.cursorY >= g.scrollY+g.visRows {
		g.scrollY = g.cursorY - g.visRows + 1
	}
}

func (g *GridModel) selectedLink() *model.Link {
	links := g.activeLinks()
	idx := g.cursorY*g.cols + g.cursorX
	if idx < 0 || idx >= len(links) {
		return nil
	}
	return &links[idx]
}

func (g *GridModel) applySearch() {
	if g.searchQuery == "" {
		g.filtered = nil
		return
	}
	q := strings.ToLower(g.searchQuery)
	g.filtered = nil
	for _, l := range g.links {
		haystack := strings.ToLower(l.Title + " " + l.URL + " " + strings.Join(l.Tags, " "))
		if strings.Contains(haystack, q) {
			g.filtered = append(g.filtered, l)
		}
	}
	g.cursorX, g.cursorY, g.scrollY = 0, 0, 0
}

func (g GridModel) Update(msg tea.Msg) (GridModel, tea.Cmd) {
	switch msg := msg.(type) {
	case GridLinksLoadedMsg:
		if msg.Err != nil {
			return g, nil
		}
		g.links = msg.Links
		g.clampCursor()
		return g, nil

	case SerendipityResultMsg:
		if msg.Err == nil && len(msg.Links) > 0 {
			g.serendipityLinks = msg.Links
			g.showSerendipity = true
		}
		return g, nil
	}

	if g.showSerendipity {
		return g.updateSerendipity(msg)
	}
	if g.searching {
		return g.updateSearch(msg)
	}
	return g.updateNormal(msg)
}

func (g GridModel) updateSerendipity(msg tea.Msg) (GridModel, tea.Cmd) {
	if msg, ok := msg.(tea.KeyPressMsg); ok {
		switch msg.String() {
		case "esc":
			g.showSerendipity = false
			g.serendipityLinks = nil
		}
	}
	return g, nil
}

func (g GridModel) updateSearch(msg tea.Msg) (GridModel, tea.Cmd) {
	if msg, ok := msg.(tea.KeyPressMsg); ok {
		switch msg.String() {
		case "esc":
			g.searching = false
			g.searchQuery = ""
			g.filtered = nil
			g.cursorX, g.cursorY, g.scrollY = 0, 0, 0
			return g, nil
		case "enter":
			g.searching = false
			return g, nil
		case "backspace":
			if len(g.searchQuery) > 0 {
				g.searchQuery = g.searchQuery[:len(g.searchQuery)-1]
				g.applySearch()
			}
			return g, nil
		default:
			r := msg.String()
			if len(r) == 1 {
				g.searchQuery += r
				g.applySearch()
			}
			return g, nil
		}
	}
	return g, nil
}

func (g GridModel) updateNormal(msg tea.Msg) (GridModel, tea.Cmd) {
	if msg, ok := msg.(tea.KeyPressMsg); ok {
		switch msg.String() {
		case "h", "left":
			g.cursorX--
			if g.cursorX < 0 {
				if g.cursorY > 0 {
					g.cursorY--
					g.cursorX = g.cols - 1
					g.clampCursor()
				} else {
					g.cursorX = 0
				}
			}
			g.ensureVisible()
		case "l", "right":
			g.cursorX++
			links := g.activeLinks()
			idx := g.cursorY*g.cols + g.cursorX
			if idx >= len(links) || g.cursorX >= g.cols {
				if g.cursorY < g.totalRows()-1 {
					g.cursorY++
					g.cursorX = 0
				} else {
					g.clampCursor()
				}
			}
			g.ensureVisible()
		case "k", "up":
			g.cursorY--
			g.clampCursor()
			g.ensureVisible()
		case "j", "down":
			g.cursorY++
			g.clampCursor()
			g.ensureVisible()
		case "/":
			g.searching = true
			g.searchQuery = ""
			return g, nil
		case "enter":
			if link := g.selectedLink(); link != nil {
				_ = exec.Command("open", link.URL).Start()
			}
		case "y":
			if link := g.selectedLink(); link != nil {
				_ = clipboard.WriteAll(link.URL)
			}
		case "r":
			return g, g.loadSerendipity
		case "esc":
			return g, func() tea.Msg { return GridExitMsg{} }
		}
	}
	return g, nil
}

func (g GridModel) View() string {
	if g.showSerendipity {
		return g.viewSerendipity()
	}

	links := g.activeLinks()
	if len(links) == 0 {
		empty := lipgloss.NewStyle().Foreground(lipgloss.Color("#9B9B9B")).Render("No saved links to display.")
		return lipgloss.Place(g.width, g.height, lipgloss.Center, lipgloss.Center, empty)
	}

	// Build grid rows
	var rows []string
	endRow := g.scrollY + g.visRows
	if endRow > g.totalRows() {
		endRow = g.totalRows()
	}

	for row := g.scrollY; row < endRow; row++ {
		var cells []string
		for col := 0; col < g.cols; col++ {
			idx := row*g.cols + col
			if idx >= len(links) {
				// pad with empty cell
				cells = append(cells, strings.Repeat(" ", g.cellW+2))
				continue
			}
			selected := row == g.cursorY && col == g.cursorX
			cells = append(cells, g.renderCell(links[idx], selected))
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
	}

	grid := lipgloss.JoinVertical(lipgloss.Left, rows...)

	// Quick Look panel
	var content string
	if g.width > quickLookMinW {
		ql := g.renderQuickLook()
		content = lipgloss.JoinHorizontal(lipgloss.Top, grid, "  ", ql)
	} else {
		content = grid
	}

	// Search bar
	var searchBar string
	if g.searching {
		searchBar = gridSearchStyle.Width(g.width).Render("/ " + g.searchQuery + "█")
	} else if g.searchQuery != "" {
		searchBar = gridSearchStyle.Width(g.width).Render(
			fmt.Sprintf("Filter: \"%s\" (%d results)", g.searchQuery, len(g.filtered)),
		)
	}

	// Status bar
	statusBar := statusBarStyle.Width(g.width).Render(
		statusTextStyle.Render("h/j/k/l") + " navigate  " +
			statusTextStyle.Render("enter") + " open  " +
			statusTextStyle.Render("y") + " copy  " +
			statusTextStyle.Render("/") + " search  " +
			statusTextStyle.Render("r") + " serendipity  " +
			statusTextStyle.Render("Esc") + " back",
	)

	padded := lipgloss.NewStyle().Margin(1, 2).Render(content)
	final := padded
	if searchBar != "" {
		final += "\n" + searchBar
	}
	final += "\n" + statusBar

	return final
}

func (g GridModel) renderCell(link model.Link, selected bool) string {
	innerW := g.cellW - 4 // border + padding

	// Color block based on first tag
	color := tagColorFromLink(link)
	block := lipgloss.NewStyle().
		Background(color).
		Width(innerW).
		Render(" ")

	// Title (truncated, up to 2 lines)
	title := link.Title
	if title == "" {
		title = link.URL
	}
	titleLines := wrapText(title, innerW)
	if len(titleLines) > 2 {
		titleLines = titleLines[:2]
		last := titleLines[1]
		if len(last) > 3 {
			titleLines[1] = last[:len(last)-3] + "..."
		}
	}
	titleStr := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFDF5")).
		Width(innerW).
		Render(strings.Join(titleLines, "\n"))

	// Compact tags
	var tagStr string
	if len(link.Tags) > 0 {
		limit := 2
		if len(link.Tags) < limit {
			limit = len(link.Tags)
		}
		var pills []string
		for _, t := range link.Tags[:limit] {
			if len(t) > 8 {
				t = t[:7] + "…"
			}
			pills = append(pills, lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFDF5")).
				Background(activeColor).
				Render(" "+t+" "))
		}
		tagStr = strings.Join(pills, " ")
	}

	cellContent := block + "\n" + titleStr + "\n" + tagStr

	style := gridNormalBorder
	if selected {
		style = gridSelectedBorder
	}

	return style.
		Width(g.cellW).
		Height(gridCellH).
		Padding(0, 1).
		Render(cellContent)
}

func (g GridModel) renderQuickLook() string {
	link := g.selectedLink()
	if link == nil {
		return gridQuickLookStyle.Width(quickLookW).Render("No selection")
	}

	innerW := quickLookW - 6
	domain := extractDomain(link.URL)
	header := domainHeaderStyle.Render(domain)

	title := cardTitleStyle.Width(innerW).Render(link.Title)
	if link.Title == "" {
		title = cardTitleStyle.Width(innerW).Render("(no title)")
	}

	displayURL := link.URL
	if len(displayURL) > innerW {
		displayURL = displayURL[:innerW-3] + "..."
	}
	urlLine := cardURLStyle.Render(displayURL)

	desc := link.Description
	if desc == "" {
		desc = "No description."
	}
	descLines := wrapText(desc, innerW)
	if len(descLines) > 4 {
		descLines = descLines[:4]
		descLines = append(descLines, "...")
	}
	descBlock := cardDescStyle.Width(innerW).Render(strings.Join(descLines, "\n"))

	var summaryBlock string
	if link.Summary != "" {
		summaryLines := wrapText(link.Summary, innerW)
		if len(summaryLines) > 3 {
			summaryLines = summaryLines[:3]
			summaryLines = append(summaryLines, "...")
		}
		summaryBlock = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#B0B0FF")).
			Italic(true).
			Width(innerW).
			Render("Summary: " + strings.Join(summaryLines, "\n"))
	}

	var tagLine string
	if len(link.Tags) > 0 {
		var pills []string
		for _, t := range link.Tags {
			pills = append(pills, tagPillStyle.Render(t))
		}
		tagLine = strings.Join(pills, " ")
	}

	dateLine := fmt.Sprintf("Added: %s", link.DateAdded.Format("2006-01-02"))

	parts := []string{header, "", title, urlLine, "", descBlock}
	if summaryBlock != "" {
		parts = append(parts, "", summaryBlock)
	}
	if tagLine != "" {
		parts = append(parts, "", tagLine)
	}
	parts = append(parts, "", dateLine)

	return gridQuickLookStyle.Width(quickLookW).Render(strings.Join(parts, "\n"))
}

func (g GridModel) viewSerendipity() string {
	var cards []string
	for i, link := range g.serendipityLinks {
		innerW := 50
		domain := extractDomain(link.URL)
		header := domainHeaderStyle.Render(domain)

		title := cardTitleStyle.Width(innerW).Render(link.Title)
		if link.Title == "" {
			title = cardTitleStyle.Width(innerW).Render("(no title)")
		}

		var summaryBlock string
		if link.Summary != "" {
			summaryLines := wrapText(link.Summary, innerW)
			if len(summaryLines) > 2 {
				summaryLines = summaryLines[:2]
				summaryLines = append(summaryLines, "...")
			}
			summaryBlock = cardDescStyle.Width(innerW).Render(strings.Join(summaryLines, "\n"))
		}

		var tagLine string
		if len(link.Tags) > 0 {
			var pills []string
			for _, t := range link.Tags {
				pills = append(pills, tagPillStyle.Render(t))
			}
			tagLine = strings.Join(pills, " ")
		}

		parts := []string{
			fmt.Sprintf("#%d", i+1),
			header,
			title,
		}
		if summaryBlock != "" {
			parts = append(parts, summaryBlock)
		}
		if tagLine != "" {
			parts = append(parts, tagLine)
		}

		cards = append(cards, serendipityCardStyle.Width(56).Render(strings.Join(parts, "\n")))
	}

	content := lipgloss.JoinVertical(lipgloss.Center, cards...)
	overlay := serendipityOverlayStyle.Render(
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFB347")).Render("✦ Serendipity Shuffle") + "\n\n" +
			content + "\n\n" +
			lipgloss.NewStyle().Foreground(lipgloss.Color("#9B9B9B")).Render("Press Esc to dismiss"),
	)

	return lipgloss.Place(g.width, g.height, lipgloss.Center, lipgloss.Center, overlay)
}

func tagColorFromLink(link model.Link) color.Color {
	tag := ""
	if len(link.Tags) > 0 {
		tag = link.Tags[0]
	}
	if tag == "" {
		// Use URL domain as fallback
		if u, err := url.Parse(link.URL); err == nil {
			tag = u.Host
		}
	}
	h := fnv.New32a()
	h.Write([]byte(tag))
	idx := int(h.Sum32()) % len(tagColors)
	return tagColors[idx]
}
