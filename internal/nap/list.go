package nap

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aquilax/truncate"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
)

// FilterValue is the snippet filter value that can be used when searching.
func (s Snippet) FilterValue() string {
	return s.Folder + "/" + s.Name + "\n" + "+" + strings.Join(s.Tags, "+") + "\n" + s.Language
}

// snippetDelegate represents the snippet list item.
type snippetDelegate struct {
	styles  SnippetsBaseStyle
	state   state
	compact bool
}

// Height is the number of lines the snippet list item takes up.
func (d snippetDelegate) Height() int {
	if d.compact {
		return 1
	}
	return 2
}

// Spacing is the number of lines to insert between list items.
func (d snippetDelegate) Spacing() int {
	if d.compact {
		return 0
	}
	return 1
}

// Update is called when the list is updated.
func (d snippetDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return nil
}

// Render renders the list item for the snippet which includes the title,
// folder, and date.
func (d snippetDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	if item == nil {
		return
	}
	s, ok := item.(Snippet)
	if !ok {
		return
	}

	titleStyle := d.styles.SelectedTitle
	subtitleStyle := d.styles.SelectedSubtitle
	if d.state == copyingState {
		titleStyle = d.styles.CopiedTitle
		subtitleStyle = d.styles.CopiedSubtitle
	} else if d.state == deletingState {
		titleStyle = d.styles.DeletedTitle
		subtitleStyle = d.styles.DeletedSubtitle
	}

	if d.compact {
		titleWidth := compactTitleWidth(m.Width())
		label := truncate.Truncate(s.Name, titleWidth, "...", truncate.PositionEnd)
		if index == m.Index() {
			fmt.Fprint(w, " "+titleStyle.Render(">"+label))
			return
		}
		fmt.Fprint(w, " "+d.styles.UnselectedTitle.Render(" "+label))
		return
	}

	title := truncate.Truncate(s.Name, snippetTitleWidth(m.Width()), "...", truncate.PositionEnd)
	subtitle := truncate.Truncate(s.Folder+" • "+humanizeTime(s.Date), snippetSubtitleWidth(m.Width()), "...", truncate.PositionEnd)
	if index == m.Index() {
		fmt.Fprintln(w, "  "+titleStyle.Render(title))
		fmt.Fprint(w, "  "+subtitleStyle.Render(subtitle))
		return
	}
	fmt.Fprintln(w, "  "+d.styles.UnselectedTitle.Render(title))
	fmt.Fprint(w, "  "+d.styles.UnselectedSubtitle.Render(subtitle))
}

// Folder represents a group of snippets in a directory.
type Folder string

// FilterValue is the searchable value for the folder.
func (f Folder) FilterValue() string {
	return string(f)
}

// folderDelegate represents a folder list item.
type folderDelegate struct {
	styles      FoldersBaseStyle
	compact     bool
	home        string
	depths      map[Folder]int
	expanded    map[Folder]bool
	children    map[Folder][]Folder
	snippets    map[Folder][]Snippet
	flashcards  map[Folder][]Snippet
	draftSource string
	draftTarget string
}

// Height is the number of lines the folder list item takes up.
func (d folderDelegate) Height() int {
	return 1
}

// Spacing is the number of lines to insert between folder items.
func (d folderDelegate) Spacing() int {
	return 0
}

// Update is what is called when the folder selection is updated.
// TODO: Update the filter search for the snippets with the folder name.
func (d folderDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return nil
}

// Render renders a folder list item.
func (d folderDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	label := d.itemLabel(item)
	if label == "" {
		return
	}
	if d.compact {
		label = truncate.Truncate(label, treeTitleWidth(m.Width()), "...", truncate.PositionEnd)
		titleWidth := compactTitleWidth(m.Width())
		label = truncate.Truncate(label, titleWidth, "...", truncate.PositionEnd)
		if index == m.Index() {
			if d.isDraftHighlighted(item) {
				fmt.Fprint(w, " "+d.styles.Selected.Render(">")+label)
				return
			}
			fmt.Fprint(w, " "+d.styles.Selected.Render(">"+label))
			return
		}
		if d.isDraftHighlighted(item) {
			fmt.Fprint(w, " "+d.styles.Unselected.Render(" ")+label)
			return
		}
		fmt.Fprint(w, " "+d.styles.Unselected.Render(" "+label))
		return
	}

	status := d.itemStatus(item)
	labelWidth := treeTitleWidth(m.Width())
	if status != "" {
		labelWidth -= lipgloss.Width(status) + 1
	}
	if labelWidth < 1 {
		labelWidth = 1
	}
	label = truncate.Truncate(label, labelWidth, "...", truncate.PositionEnd)
	label = d.highlightSnippetLabel(item, label)

	style := d.styles.Unselected
	prefix := "  "
	if index == m.Index() {
		style = d.styles.Selected
		prefix = "→ "
	}

	fmt.Fprint(w, "  ")
	if d.isDraftHighlighted(item) {
		fmt.Fprint(w, style.Render(prefix))
		fmt.Fprint(w, label)
	} else {
		fmt.Fprint(w, style.Render(prefix+label))
	}
	if status == "" {
		return
	}

	gapWidth := treeTitleWidth(m.Width()) - lipgloss.Width(label) - lipgloss.Width(status)
	if gapWidth < 1 {
		gapWidth = 1
	}
	fmt.Fprint(w, strings.Repeat(" ", gapWidth))
	fmt.Fprint(w, status)
}

func (d folderDelegate) itemLabel(item list.Item) string {
	switch v := item.(type) {
	case Folder:
		depth := d.depths[v]
		return strings.Repeat("  ", depth) + folderIndicator(d.children, d.snippets, d.expanded, v) + folderLabel(v)
	case Snippet:
		depth := d.depths[Folder(v.Folder)] + 1
		return strings.Repeat("  ", depth) + "• " + v.Name
	default:
		return ""
	}
}

func (d folderDelegate) itemStatus(item list.Item) string {
	folder, ok := item.(Folder)
	if !ok {
		return ""
	}

	states := descendantFlashcardStates(d.home, d.children, d.flashcards, folder, time.Now())
	if len(states) == 0 {
		return ""
	}

	dots := make([]string, 0, len(states))
	for _, state := range states {
		switch state {
		case flashcardDeckRecall:
			dots = append(dots, d.styles.FlashcardRecall.Render("●"))
		case flashcardDeckPositive:
			dots = append(dots, d.styles.FlashcardPositive.Render("●"))
		case flashcardDeckNegative:
			dots = append(dots, d.styles.FlashcardNegative.Render("●"))
		}
	}

	return strings.Join(dots, " ")
}

func (d folderDelegate) highlightSnippetLabel(item list.Item, label string) string {
	snippet, ok := item.(Snippet)
	if !ok {
		return label
	}
	switch snippet.Path() {
	case d.draftSource:
		return d.styles.FlashcardPending.Render(label)
	case d.draftTarget:
		return d.styles.FlashcardPositive.Render(label)
	default:
		return label
	}
}

func (d folderDelegate) isDraftHighlighted(item list.Item) bool {
	snippet, ok := item.(Snippet)
	return ok && (snippet.Path() == d.draftSource || snippet.Path() == d.draftTarget)
}

const (
	Day   = 24 * time.Hour
	Week  = 7 * Day
	Month = 30 * Day
	Year  = 12 * Month
)

var magnitudes = []humanize.RelTimeMagnitude{
	{D: 5 * time.Second, Format: "just now", DivBy: time.Second},
	{D: time.Minute, Format: "moments ago", DivBy: time.Second},
	{D: time.Hour, Format: "%dm %s", DivBy: time.Minute},
	{D: 2 * time.Hour, Format: "1h %s", DivBy: 1},
	{D: Day, Format: "%dh %s", DivBy: time.Hour},
	{D: 2 * Day, Format: "1d %s", DivBy: 1},
	{D: Week, Format: "%dd %s", DivBy: Day},
	{D: 2 * Week, Format: "1w %s", DivBy: 1},
	{D: Month, Format: "%dw %s", DivBy: Week},
	{D: 2 * Month, Format: "1mo %s", DivBy: 1},
	{D: Year, Format: "%dmo %s", DivBy: Month},
	{D: 18 * Month, Format: "1y %s", DivBy: 1},
	{D: 2 * Year, Format: "2y %s", DivBy: 1},
}

func humanizeTime(t time.Time) string {
	return humanize.CustomRelTime(t, time.Now(), "ago", "from now", magnitudes)
}

func compactTitleWidth(width int) int {
	if width <= 7 {
		return 4
	}
	return width - 4
}

func treeTitleWidth(width int) int {
	if width <= 9 {
		return 4
	}
	return width - 6
}

func snippetTitleWidth(width int) int {
	if width <= 7 {
		return 4
	}
	return width - 2
}

func snippetSubtitleWidth(width int) int {
	if width <= 7 {
		return 4
	}
	return width - 2
}

func folderLabel(folder Folder) string {
	value := string(folder)
	if idx := strings.LastIndex(value, "/"); idx >= 0 {
		return value[idx+1:]
	}
	return value
}

func folderIndicator(children map[Folder][]Folder, snippets map[Folder][]Snippet, expanded map[Folder]bool, folder Folder) string {
	if len(children[folder]) == 0 && len(snippets[folder]) == 0 {
		return "• "
	}
	if expanded[folder] {
		return "▾ "
	}
	return "▸ "
}

func descendantFlashcardStates(home string, children map[Folder][]Folder, flashcards map[Folder][]Snippet, folder Folder, now time.Time) []flashcardDeckState {
	states := make([]flashcardDeckState, 0, len(flashcards[folder]))

	var walk func(Folder)
	walk = func(current Folder) {
		for _, snippet := range flashcards[current] {
			if isNativeFlashcardDeck(snippet) {
				states = append(states, nativeFlashcardIndicatorStates(home, snippet, now)...)
				continue
			}
			state, ok := flashcardDeckStateForSnippet(snippet)
			if !ok {
				continue
			}
			if state == flashcardDeckPending {
				continue
			}
			states = append(states, state)
		}

		for _, child := range children[current] {
			walk(child)
		}
	}

	walk(folder)
	return states
}
