package ui

import (
	"database/sql"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/alexzajac/the-dredger/internal/db"
	"github.com/alexzajac/the-dredger/internal/model"
)

type FocusContext int

const (
	focusPending FocusContext = iota
	focusSaved
)

type UndoFrame struct {
	Link   model.Link
	Action string
}

type FocusModel struct {
	db      *sql.DB
	current *model.Link
	next    *model.Link
	context FocusContext

	descScroll    int
	descMaxScroll int

	anim AnimState

	undoStack []UndoFrame

	tagging  bool
	tagInput textinput.Model

	width, height int

	kept, pruned, snoozed int

	startLink *model.Link
}

func NewFocusModel(database *sql.DB, width, height int, ctx FocusContext, startLink *model.Link) FocusModel {
	ti := textinput.New()
	ti.Placeholder = "add tag..."
	ti.CharLimit = 40

	return FocusModel{
		db:        database,
		anim:      newAnimState(),
		tagInput:  ti,
		width:     width,
		height:    height,
		context:   ctx,
		startLink: startLink,
	}
}

func (f FocusModel) loadNextLink() tea.Msg {
	var link *model.Link
	var err error
	if f.context == focusSaved {
		link, err = db.GetNextSaved(f.db)
	} else {
		link, err = db.GetNextUnprocessed(f.db)
	}
	return NextLinkLoadedMsg{Link: link, Err: err}
}

func (f FocusModel) prefetchNextLink() tea.Msg {
	var link *model.Link
	if f.context == focusSaved {
		link, _ = db.GetNextSaved(f.db)
	} else {
		link, _ = db.GetNextUnprocessed(f.db)
	}
	return NextLinkPrefetchedMsg{Link: link}
}

func (f FocusModel) Init() tea.Cmd {
	if f.startLink != nil {
		link := f.startLink
		return func() tea.Msg {
			return NextLinkLoadedMsg{Link: link}
		}
	}
	return f.loadNextLink
}

func (f FocusModel) Update(msg tea.Msg) (FocusModel, tea.Cmd) {
	switch msg := msg.(type) {
	case NextLinkLoadedMsg:
		if msg.Err != nil {
			return f, nil
		}
		f.current = msg.Link
		f.descScroll = 0
		if f.current != nil {
			return f, f.prefetchNextLink
		}
		return f, nil

	case NextLinkPrefetchedMsg:
		f.next = msg.Link
		return f, nil

	case AnimTickMsg:
		if !f.anim.active {
			return f, nil
		}
		f.anim.tick()
		if f.anim.done {
			f.current = f.next
			f.next = nil
			f.descScroll = 0
			if f.current != nil {
				return f, f.prefetchNextLink
			}
			return f, nil
		}
		return f, animTick()

	case DredgeLinkResultMsg:
		if f.current != nil && f.current.ID == msg.LinkID {
			f.current.DredgeState = msg.State
			f.current.DredgeError = msg.Error
			if msg.Title != "" {
				f.current.Title = msg.Title
			}
			if msg.Description != "" {
				f.current.Description = msg.Description
			}
			if msg.Summary != "" {
				f.current.Summary = msg.Summary
			}
			if len(msg.Tags) > 0 {
				f.current.Tags = msg.Tags
			}
		}
		return f, nil

	case tea.KeyPressMsg:
		if f.tagging {
			return f.updateTagging(msg)
		}
		return f.updateNormal(msg)
	}

	return f, nil
}

func (f FocusModel) updateTagging(msg tea.KeyPressMsg) (FocusModel, tea.Cmd) {
	switch msg.String() {
	case "enter":
		tag := strings.TrimSpace(f.tagInput.Value())
		if tag != "" && f.current != nil {
			f.current.Tags = append(f.current.Tags, tag)
			_ = db.UpdateLink(f.db, *f.current)
		}
		f.tagging = false
		f.tagInput.Reset()
		return f, nil
	case "esc":
		f.tagging = false
		f.tagInput.Reset()
		return f, nil
	}

	var cmd tea.Cmd
	f.tagInput, cmd = f.tagInput.Update(msg)
	return f, cmd
}

func (f FocusModel) updateNormal(msg tea.KeyPressMsg) (FocusModel, tea.Cmd) {
	if f.anim.active {
		return f, nil
	}

	switch msg.String() {
	case "h":
		if f.current == nil {
			return f, nil
		}
		if f.context == focusSaved {
			// Saved context: [h] moves to pending (unprocessed)
			f.undoStack = append(f.undoStack, UndoFrame{
				Link:   *f.current,
				Action: "moved to pending",
			})
			f.current.Status = model.Unprocessed
			_ = db.UpdateLink(f.db, *f.current)
			f.anim.start(-80, snoozeColor)
			return f, animTick()
		}
		// Pending context: [h] prunes
		f.undoStack = append(f.undoStack, UndoFrame{
			Link:   *f.current,
			Action: "pruned",
		})
		f.current.Status = model.Pruned
		_ = db.UpdateLink(f.db, *f.current)
		f.pruned++
		f.anim.start(-80, pruneColor)
		return f, animTick()

	case "l":
		if f.current == nil || f.context == focusSaved {
			return f, nil
		}
		f.undoStack = append(f.undoStack, UndoFrame{
			Link:   *f.current,
			Action: "kept",
		})
		f.current.Status = model.Saved
		_ = db.UpdateLink(f.db, *f.current)
		f.kept++
		f.anim.start(80, keepColor)
		return f, animTick()

	case "s":
		if f.current == nil || f.context == focusSaved {
			return f, nil
		}
		f.undoStack = append(f.undoStack, UndoFrame{
			Link:   *f.current,
			Action: "snoozed",
		})
		snoozeUntil := time.Now().Add(7 * 24 * time.Hour)
		_ = db.SnoozeLink(f.db, f.current.ID, snoozeUntil)
		f.snoozed++
		f.anim.start(80, snoozeColor)
		return f, animTick()

	case "t":
		if f.current == nil {
			return f, nil
		}
		f.tagging = true
		cmd := f.tagInput.Focus()
		return f, cmd

	case "r":
		if f.current == nil || f.context != focusSaved {
			return f, nil
		}
		// Open URL in default browser
		_ = exec.Command("open", f.current.URL).Start()
		return f, nil

	case "d":
		if f.current == nil || f.context != focusSaved {
			return f, nil
		}
		return f, func() tea.Msg {
			return TriggerDredgeLinkMsg{LinkID: f.current.ID, URL: f.current.URL}
		}

	case "j":
		f.descScroll++
		if f.descScroll > f.descMaxScroll {
			f.descScroll = f.descMaxScroll
		}
		return f, nil

	case "k":
		f.descScroll--
		if f.descScroll < 0 {
			f.descScroll = 0
		}
		return f, nil

	case "z":
		if len(f.undoStack) == 0 {
			return f, nil
		}
		// Pop last undo frame
		frame := f.undoStack[len(f.undoStack)-1]
		f.undoStack = f.undoStack[:len(f.undoStack)-1]

		// Restore the link to its previous state
		_ = db.RestoreLink(f.db, frame.Link)
		restored := frame.Link
		f.current = &restored

		switch frame.Action {
		case "kept":
			f.kept--
		case "pruned":
			f.pruned--
		case "snoozed":
			f.snoozed--
		}

		f.anim.active = false
		f.anim.done = false
		f.anim.offsetX = 0
		f.descScroll = 0
		return f, f.prefetchNextLink

	case "esc":
		return f, func() tea.Msg { return FocusExitMsg{} }
	}

	return f, nil
}

func (f FocusModel) View() string {
	if f.current == nil && !f.anim.active {
		return f.viewCompletion()
	}

	card := f.renderCard()

	// Help line — context-aware
	var help string
	if f.context == focusSaved {
		help = lipgloss.NewStyle().Foreground(lipgloss.Color("#9B9B9B")).Render(
			statusTextStyle.Render("h") + " pending  " +
				statusTextStyle.Render("t") + " tag  " +
				statusTextStyle.Render("d") + " dredge  " +
				statusTextStyle.Render("r") + " read  " +
				statusTextStyle.Render("z") + " undo  " +
				statusTextStyle.Render("Esc") + " back",
		)
	} else {
		help = lipgloss.NewStyle().Foreground(lipgloss.Color("#9B9B9B")).Render(
			statusTextStyle.Render("h") + " prune  " +
				statusTextStyle.Render("l") + " keep  " +
				statusTextStyle.Render("s") + " snooze  " +
				statusTextStyle.Render("t") + " tag  " +
				statusTextStyle.Render("z") + " undo  " +
				statusTextStyle.Render("Esc") + " back",
		)
	}

	// Stats line
	stats := lipgloss.NewStyle().Foreground(lipgloss.Color("#9B9B9B")).Render(
		fmt.Sprintf("Kept: %d | Pruned: %d | Snoozed: %d", f.kept, f.pruned, f.snoozed),
	)

	// Undo toast — show while stack is non-empty
	var undo string
	if len(f.undoStack) > 0 {
		last := f.undoStack[len(f.undoStack)-1]
		title := last.Link.Title
		if len(title) > 25 {
			title = title[:22] + "..."
		}
		if title == "" {
			title = last.Link.URL
			if len(title) > 25 {
				title = title[:22] + "..."
			}
		}
		undo = "\n" + undoToastStyle.Render(
			fmt.Sprintf("↩ Undo %s \"%s\" (z) [%d in stack]", last.Action, title, len(f.undoStack)),
		)
	}

	// Tag input
	var tagLine string
	if f.tagging {
		tagLine = "\n" + f.tagInput.View()
	}

	content := card + tagLine + "\n\n" + help + "\n\n" + stats + undo

	return lipgloss.Place(f.width, f.height, lipgloss.Center, lipgloss.Center, content)
}

func (f *FocusModel) renderCard() string {
	link := f.current
	if link == nil {
		return ""
	}

	cardWidth := max(30, min(64, f.width-20))
	innerWidth := cardWidth - 6 // account for border + padding

	// Domain header
	domain := extractDomain(link.URL)
	header := domainHeaderStyle.Render(domain)

	// Title
	title := cardTitleStyle.Width(innerWidth).Render(link.Title)
	if link.Title == "" {
		title = cardTitleStyle.Width(innerWidth).Render("(no title)")
	}

	// URL (truncated)
	displayURL := link.URL
	if len(displayURL) > innerWidth {
		displayURL = displayURL[:innerWidth-3] + "..."
	}
	urlLine := cardURLStyle.Render(displayURL)

	// Description with scrolling
	desc := link.Description
	if desc == "" {
		desc = "No description available."
	}
	descLines := wrapText(desc, innerWidth)
	maxVisible := 6
	f.descMaxScroll = max(0, len(descLines)-maxVisible)
	if f.descScroll > f.descMaxScroll {
		f.descScroll = f.descMaxScroll
	}
	end := min(f.descScroll+maxVisible, len(descLines))
	visibleDesc := strings.Join(descLines[f.descScroll:end], "\n")
	descBlock := cardDescStyle.Width(innerWidth).Render(visibleDesc)

	// Scroll indicator
	var scrollHint string
	if f.descMaxScroll > 0 {
		scrollHint = lipgloss.NewStyle().Foreground(lipgloss.Color("#626262")).Render(
			fmt.Sprintf(" ↕ %d/%d", f.descScroll+1, f.descMaxScroll+1),
		)
	}

	// Dredge state badge
	var dredgeBadge string
	if link.DredgeState != model.DredgeNone {
		badgeStyle := lipgloss.NewStyle().Padding(0, 1)
		switch link.DredgeState {
		case model.DredgeCrawling, model.DredgeCrunching:
			badgeStyle = badgeStyle.Foreground(lipgloss.Color("#FFB347"))
		case model.DredgeComplete:
			badgeStyle = badgeStyle.Foreground(keepColor)
		case model.DredgeCapsized:
			badgeStyle = badgeStyle.Foreground(pruneColor)
		}
		label := link.DredgeState.String()
		if link.DredgeState == model.DredgeCapsized && link.DredgeError != "" {
			errLines := wrapText(link.DredgeError, innerWidth-2)
			label = "Failed: " + strings.Join(errLines, "\n")
		}
		dredgeBadge = badgeStyle.Render(label)
	}

	// Summary
	var summaryBlock string
	if link.Summary != "" {
		summaryBlock = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#B0B0FF")).
			Italic(true).
			Width(innerWidth).
			Render("Summary: " + link.Summary)
	}

	// Tags
	var tagLine string
	if len(link.Tags) > 0 {
		var pills []string
		for _, t := range link.Tags {
			pills = append(pills, tagPillStyle.Render(t))
		}
		tagLine = strings.Join(pills, " ")
	}

	// Date
	dateLine := fmt.Sprintf("Added: %s", link.DateAdded.Format("2006-01-02"))

	// Status indicator
	statusLabel := lipgloss.NewStyle().
		Foreground(lipgloss.Color(link.Status.Color())).
		Bold(true).
		Render(strings.ToUpper(link.Status.String()))

	// Assemble card content
	parts := []string{header, "", title, urlLine, "", descBlock + scrollHint}
	if summaryBlock != "" {
		parts = append(parts, "", summaryBlock)
	}
	if dredgeBadge != "" {
		parts = append(parts, dredgeBadge)
	}
	if tagLine != "" {
		parts = append(parts, "", tagLine)
	}
	parts = append(parts, statusLabel+"  "+dateLine)

	cardContent := strings.Join(parts, "\n")

	borderStyle := cardBorderStyle.Width(cardWidth)
	if f.anim.active && f.anim.flashFrames > 0 {
		borderStyle = borderStyle.BorderForeground(f.anim.flashColor)
	}

	return borderStyle.Render(cardContent)
}

func (f FocusModel) viewCompletion() string {
	var message string
	if f.context == focusSaved {
		message = "No saved bookmarks yet.\n\n" +
			lipgloss.NewStyle().Foreground(lipgloss.Color("#9B9B9B")).Render("Press Esc to return")
	} else {
		message = "All caught up!\n\n" +
			fmt.Sprintf("Kept: %d | Pruned: %d | Snoozed: %d\n\n", f.kept, f.pruned, f.snoozed) +
			lipgloss.NewStyle().Foreground(lipgloss.Color("#9B9B9B")).Render("Press [b] to view saved bookmarks  |  Esc to return")
	}

	msg := completionStyle.Width(50).Render(message)
	return lipgloss.Place(f.width, f.height, lipgloss.Center, lipgloss.Center, msg)
}

func extractDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return rawURL
	}
	return u.Host
}

func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}

	var lines []string
	current := words[0]
	for _, w := range words[1:] {
		if len(current)+1+len(w) > width {
			lines = append(lines, current)
			current = w
		} else {
			current += " " + w
		}
	}
	lines = append(lines, current)
	return lines
}
