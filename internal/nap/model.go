package nap

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2/quick"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	maxPane = 3

	expandedSnippetPaneWidth  = 35
	expandedFolderPaneWidth   = 22
	collapsedSnippetPaneWidth = 12
	collapsedFolderPaneWidth  = 10
	minContentPaneWidth       = 20
	previewWidthOffset        = 8
)

type pane int

const (
	snippetPane pane = iota
	contentPane
	folderPane
)

type state int

const (
	navigatingState state = iota
	deletingState
	creatingState
	copyingState
	pastingState
	quittingState
	editingState
	editingTagsState
)

type input int

const (
	folderInput input = iota
	nameInput
	languageInput
)

// Model represents the state of the application.
// It contains all the snippets organized in folders.
type Model struct {
	// the config map.
	config Config
	// the key map.
	keys KeyMap
	// the help model.
	help help.Model
	// the height of the terminal.
	height int
	width  int
	// the working directory.
	Workdir string
	// the List of snippets to display to the user.
	Lists map[Folder]*list.Model
	// the list of Folders to display to the user.
	Folders list.Model
	// folder tree metadata for the folder pane.
	folderTree     folderTree
	folderExpanded map[Folder]bool
	// the viewport of the Code snippet.
	Code        viewport.Model
	LineNumbers viewport.Model
	// the input for snippet folder, name, language
	activeInput input
	inputs      []textinput.Model
	tagsInput   textinput.Model
	// the current active pane of focus.
	pane pane
	// the current state / action of the application.
	state state
	// stying for components
	ListStyle    SnippetsBaseStyle
	FoldersStyle FoldersBaseStyle
	ContentStyle ContentBaseStyle
	contentCache map[contentCacheKey]contentCacheEntry
}

// Init initialzes the application model.
func (m *Model) Init() tea.Cmd {
	rand.Seed(time.Now().Unix())
	if m.contentCache == nil {
		m.contentCache = map[contentCacheKey]contentCacheEntry{}
	}
	if m.folderExpanded == nil {
		m.folderExpanded = map[Folder]bool{}
	}
	m.rebuildFolderTree()

	m.Folders.Styles.Title = m.FoldersStyle.Title
	m.Folders.Styles.TitleBar = m.FoldersStyle.TitleBar
	m.updateKeyMap()

	return m.updateContent()
}

type contentCacheKey struct {
	path     string
	width    int
	theme    string
	markdown bool
}

type contentCacheEntry struct {
	modTimeUnixNano int64
	size            int64
	rendered        string
	lineCount       int
}

type contentRenderedMsg struct {
	snippet           Snippet
	width             int
	rendered          string
	lineCount         int
	showCreateHint    bool
	showNoContentHint bool
	err               error
	previewerMissing  bool
	cacheKey          contentCacheKey
	modTimeUnixNano   int64
	size              int64
}

type refreshContentMsg struct{}

func (m *Model) contentKey(snippet Snippet, width int) contentCacheKey {
	return contentCacheKey{
		path:     snippet.Path(),
		width:    width,
		theme:    m.config.Theme,
		markdown: isMarkdownLanguage(snippet.Language),
	}
}

func (m *Model) contentWidth(snippet Snippet) int {
	if isMarkdownLanguage(snippet.Language) {
		return m.previewWidth()
	}
	return 0
}

func (m *Model) cachedContent(snippet Snippet, width int) (contentRenderedMsg, bool) {
	if m.contentCache == nil {
		return contentRenderedMsg{}, false
	}

	path, err := snippetStoragePath(m.config.Home, snippet)
	if err != nil {
		return contentRenderedMsg{}, false
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return contentRenderedMsg{}, false
	}

	key := m.contentKey(snippet, width)
	entry, ok := m.contentCache[key]
	if !ok {
		return contentRenderedMsg{}, false
	}

	modTimeUnixNano := info.ModTime().UnixNano()
	if entry.modTimeUnixNano != modTimeUnixNano || entry.size != info.Size() {
		return contentRenderedMsg{}, false
	}

	return contentRenderedMsg{
		snippet:         snippet,
		width:           width,
		rendered:        entry.rendered,
		lineCount:       entry.lineCount,
		cacheKey:        key,
		modTimeUnixNano: entry.modTimeUnixNano,
		size:            entry.size,
	}, true
}

func renderContent(config Config, snippet Snippet, width int, key contentCacheKey) tea.Msg {
	path, err := snippetStoragePath(config.Home, snippet)
	if err != nil {
		return contentRenderedMsg{snippet: snippet, width: width, showNoContentHint: true}
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return contentRenderedMsg{snippet: snippet, width: width, showNoContentHint: true}
	}

	content, err := os.ReadFile(path)
	if err != nil || len(content) == 0 {
		return contentRenderedMsg{snippet: snippet, width: width, showNoContentHint: true}
	}

	msg := contentRenderedMsg{
		snippet:         snippet,
		width:           width,
		cacheKey:        key,
		modTimeUnixNano: info.ModTime().UnixNano(),
		size:            info.Size(),
	}

	if isMarkdownLanguage(snippet.Language) {
		rendered, err := previewContent(string(content), width)
		if err != nil {
			msg.err = err
			msg.previewerMissing = err == errPreviewerNotFound
			return msg
		}
		msg.rendered = rendered
		msg.lineCount = lipgloss.Height(rendered)
		return msg
	}

	var b bytes.Buffer
	err = quick.Highlight(&b, string(content), snippet.Language, "terminal16m", config.Theme)
	if err != nil {
		msg.err = fmt.Errorf("Unable to highlight file.")
		return msg
	}

	msg.rendered = b.String()
	msg.lineCount = lipgloss.Height(msg.rendered)
	return msg
}

// updateContent instructs the application to fetch the latest contents of the
// snippet file.
//
// This is useful after a Paste or Edit.
func (m *Model) updateContent() tea.Cmd {
	if len(m.List().Items()) <= 0 {
		return func() tea.Msg {
			return contentRenderedMsg{showCreateHint: true}
		}
	}

	snippet := m.selectedSnippet()
	width := m.contentWidth(snippet)
	if msg, ok := m.cachedContent(snippet, width); ok {
		return func() tea.Msg {
			return msg
		}
	}

	config := m.config
	key := m.contentKey(snippet, width)
	return func() tea.Msg {
		return renderContent(config, snippet, width, key)
	}
}

type updateFoldersMsg struct {
	items               []list.Item
	selectedFolderIndex int
	refreshContent      bool
}

// updateFolders returns a Cmd to  tell the application that there are possible
// folder changes to update.
func (m *Model) updateFolders() tea.Cmd {
	return m.updateFoldersForSelection(m.selectedFolder(), false)
}

func (m *Model) updateFoldersForSelection(selected Folder, refreshContent bool) tea.Cmd {
	return func() tea.Msg {
		return m.updateFoldersView(selected, refreshContent)
	}
}

// changeStateMsg tells the application to enter a different state.
type changeStateMsg struct{ newState state }

// changeState returns a Cmd to enter a different state.
func changeState(newState state) tea.Cmd {
	return func() tea.Msg {
		return changeStateMsg{newState}
	}
}

// Update updates the model based on user interaction.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case updateFoldersMsg:
		setItemsCmd := m.Folders.SetItems(msg.items)
		m.Folders.Select(msg.selectedFolderIndex)
		var cmd tea.Cmd
		m.Folders, cmd = m.Folders.Update(msg)
		cmds := []tea.Cmd{setItemsCmd, cmd}
		if msg.refreshContent {
			cmds = append(cmds, m.updateContent())
		}
		return m, tea.Batch(cmds...)
	case contentRenderedMsg:
		return m.applyContentView(msg)
	case refreshContentMsg:
		return m, m.updateContent()
	case changeStateMsg:
		m.List().SetDelegate(snippetDelegate{styles: m.ListStyle, state: msg.newState, compact: m.isCollapsedPreview()})

		var cmd tea.Cmd

		if m.state == msg.newState {
			break
		}

		wasEditing := m.state == editingState
		wasPasting := m.state == pastingState
		wasCreating := m.state == creatingState
		m.state = msg.newState
		m.updateKeyMap()
		m.updateActivePane(msg)

		switch msg.newState {
		case navigatingState:
			if wasPasting || wasCreating {
				return m, m.updateContent()
			}

			if wasEditing {
				m.blurInputs()
				i := m.List().Index()
				snippet := m.selectedSnippet()
				if m.inputs[nameInput].Value() != "" {
					snippet.Name = m.inputs[nameInput].Value()
				} else {
					snippet.Name = defaultSnippetName
				}
				if m.inputs[folderInput].Value() != "" {
					snippet.Folder = m.inputs[folderInput].Value()
				} else {
					snippet.Folder = defaultSnippetFolder
				}
				if m.inputs[languageInput].Value() != "" {
					snippet.Language = m.inputs[languageInput].Value()
				} else {
					snippet.Language = m.config.DefaultLanguage
				}
				file := fmt.Sprintf("%s.%s", snippet.Name, snippet.Language)
				snippet.File = file
				newPath, err := snippetStoragePath(m.config.Home, snippet)
				if err != nil {
					m.state = editingState
					m.displayError(err.Error())
					return m, m.focusInput(folderInput)
				}
				_ = os.MkdirAll(filepath.Dir(newPath), os.ModePerm)
				_ = os.Rename(m.selectedSnippetFilePath(), newPath)
				setCmd := m.List().SetItem(i, snippet)
				m.pane = snippetPane
				cmd = tea.Batch(setCmd, m.updateFoldersForSelection(Folder(snippet.Folder), false), m.updateContent())
			}
		case pastingState:
			content, err := clipboard.ReadAll()
			if err != nil {
				return m, changeState(navigatingState)
			}
			f, err := os.OpenFile(m.selectedSnippetFilePath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
			if err != nil {
				return m, changeState(navigatingState)
			}
			defer f.Close()
			f.WriteString(content)
			return m, changeState(navigatingState)
		case deletingState:
			m.state = deletingState
		case editingState:
			m.pane = contentPane
			snippet := m.selectedSnippet()
			m.inputs[folderInput].SetValue(snippet.Folder)
			if snippet.Name == defaultSnippetName {
				m.inputs[nameInput].SetValue("")
			} else {
				m.inputs[nameInput].SetValue(snippet.Name)
			}
			m.inputs[languageInput].SetValue(snippet.Language)
			cmd = m.focusInput(m.activeInput)
		case creatingState:
		case copyingState:
			m.pane = snippetPane
			m.state = copyingState
			m.updateActivePane(msg)
			cmd = tea.Tick(time.Second, func(t time.Time) tea.Msg {
				return changeStateMsg{navigatingState}
			})
		}

		m.updateKeyMap()
		m.updateActivePane(msg)
		return m, cmd
	case tea.WindowSizeMsg:
		previousCodeWidth := m.Code.Width
		m.width = msg.Width
		m.height = msg.Height - 4
		for _, li := range m.Lists {
			li.SetHeight(m.height)
		}
		m.Folders.SetHeight(m.height)
		m.Code.Height = m.height
		m.LineNumbers.Height = m.height
		m.updatePaneLayout(msg.Width)
		if m.Code.Width != previousCodeWidth {
			return m, m.updateContent()
		}
		return m, nil
	case tea.KeyMsg:
		if m.List().FilterState() == list.Filtering {
			break
		}

		if m.state == deletingState {
			switch {
			case key.Matches(msg, m.keys.Confirm):
				_ = os.Remove(m.selectedSnippetFilePath())
				m.List().RemoveItem(m.List().Index())
				m.state = navigatingState
				m.updateKeyMap()
				return m, tea.Batch(changeState(navigatingState), m.updateFolders(), m.updateContent())
			case key.Matches(msg, m.keys.Quit, m.keys.Cancel):
				return m, changeState(navigatingState)
			}
			return m, nil
		} else if m.state == copyingState {
			return m, changeState(navigatingState)
		} else if m.state == editingState {
			if msg.String() == "esc" || msg.String() == "enter" {
				return m, changeState(navigatingState)
			}
			var cmd tea.Cmd
			var cmds []tea.Cmd
			for i := range m.inputs {
				m.inputs[i], cmd = m.inputs[i].Update(msg)
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}

		if cmd, ok := m.updateFolderTreeNavigation(msg); ok {
			return m, cmd
		}

		switch {
		case key.Matches(msg, m.keys.NextPane):
			m.nextPane()
		case key.Matches(msg, m.keys.PreviousPane):
			m.previousPane()
		case key.Matches(msg, m.keys.Quit):
			m.saveState()
			m.state = quittingState
			return m, tea.Quit
		case key.Matches(msg, m.keys.NewSnippet):
			m.state = creatingState
			return m, m.createNewSnippetFile()
		case key.Matches(msg, m.keys.MoveSnippetDown):
			m.moveSnippetDown()
		case key.Matches(msg, m.keys.MoveSnippetUp):
			m.moveSnippetUp()
		case key.Matches(msg, m.keys.PasteSnippet):
			return m, changeState(pastingState)
		case key.Matches(msg, m.keys.RenameSnippet):
			m.activeInput = nameInput
			return m, changeState(editingState)
		case key.Matches(msg, m.keys.ChangeFolder):
			m.pane = snippetPane
			cmd := m.updateActivePane(msg)
			return m, cmd
		case key.Matches(msg, m.keys.ToggleHelp):
			m.help.ShowAll = !m.help.ShowAll

			var newHeight int
			if m.help.ShowAll {
				newHeight = m.height - 4
			} else {
				newHeight = m.height
			}
			m.List().SetHeight(newHeight)
			m.Folders.SetHeight(newHeight)
			m.Code.Height = newHeight
			m.LineNumbers.Height = newHeight
		case key.Matches(msg, m.keys.SetFolder):
			m.activeInput = folderInput
			return m, changeState(editingState)
		case key.Matches(msg, m.keys.SetLanguage):
			m.activeInput = languageInput
			return m, changeState(editingState)
		case key.Matches(msg, m.keys.CopySnippet):
			return m, func() tea.Msg {
				content, err := os.ReadFile(m.selectedSnippetFilePath())
				if err != nil {
					return changeStateMsg{navigatingState}
				}
				clipboard.WriteAll(string(content))
				return changeStateMsg{copyingState}
			}
		case key.Matches(msg, m.keys.DeleteSnippet):
			m.pane = snippetPane
			m.updateActivePane(msg)
			m.List().Title = "Delete? (y/N)"
			return m, changeState(deletingState)
		case key.Matches(msg, m.keys.EditSnippet):
			return m, m.editSnippet()
		case key.Matches(msg, m.keys.Search):
			m.pane = snippetPane
		}
	case tea.MouseMsg:
		if cmd, ok := m.updateContentScroll(msg); ok {
			return m, cmd
		}
	}

	if cmd, ok := m.updateContentScroll(msg); ok {
		return m, cmd
	}

	m.updateKeyMap()
	cmd := m.updateActivePane(msg)
	return m, cmd
}

// blurInputs blurs all the inputs.
func (m *Model) blurInputs() {
	for i := range m.inputs {
		m.inputs[i].Blur()
	}
}

// focusInput focuses the speficied input and blurs the rest.
func (m *Model) focusInput(i input) tea.Cmd {
	m.blurInputs()
	m.inputs[i].CursorEnd()
	return m.inputs[i].Focus()
}

// selectedSnippetFilePath returns the file path of the snippet that is
// currently selected.
func (m *Model) selectedSnippetFilePath() string {
	path, err := snippetStoragePath(m.config.Home, m.selectedSnippet())
	if err != nil {
		return ""
	}
	return path
}

// nextPane sets the next pane to be active.
func (m *Model) nextPane() {
	switch m.pane {
	case folderPane:
		m.pane = snippetPane
	case snippetPane:
		m.pane = contentPane
	}
}

// previousPane sets the previous pane to be active.
func (m *Model) previousPane() {
	switch m.pane {
	case contentPane:
		m.pane = snippetPane
	case snippetPane:
		m.pane = folderPane
	}
}

// editSnippet opens the editor with the selected snippet file path.
func (m *Model) editSnippet() tea.Cmd {
	return tea.ExecProcess(editorCmd(m.selectedSnippetFilePath()), func(err error) tea.Msg {
		return refreshContentMsg{}
	})
}

func (m *Model) noContentHints() []keyHint {
	return []keyHint{
		{m.keys.EditSnippet, "edit contents"},
		{m.keys.PasteSnippet, "paste clipboard"},
		{m.keys.RenameSnippet, "rename"},
		{m.keys.SetFolder, "set folder"},
		{m.keys.SetLanguage, "set language"},
	}
}

// updateFolderView updates the folders list to display the current folders.
func (m *Model) updateFoldersView(selectedFolder Folder, refreshContent bool) tea.Msg {
	for folder, li := range m.Lists {
		for i := 0; i < len(li.Items()); {
			item := li.Items()[i]
			snippet, ok := item.(Snippet)
			if !ok {
				i++
				continue
			}
			f := Folder(snippet.Folder)
			_, ok = m.Lists[f]
			if !ok {
				m.Lists[f] = newList([]list.Item{}, m.height, m.ListStyle)
			}
			if f != folder {
				li.RemoveItem(i)
				m.Lists[f].InsertItem(0, item)
				continue
			}
			i++
		}
	}

	m.rebuildFolderTree()
	if selectedFolder == "" {
		selectedFolder = m.selectedFolder()
	}
	m.revealFolder(selectedFolder)
	folderItems := m.folderTree.visibleItems(m.folderExpanded)
	selectedFolderIndex := visibleFolderIndex(folderItems, selectedFolder, m.folderTree.parents)

	return updateFoldersMsg{
		items:               folderItems,
		selectedFolderIndex: selectedFolderIndex,
		refreshContent:      refreshContent,
	}
}

func (m *Model) rebuildFolderTree() {
	ensureAncestorLists(m.Lists, m.height, m.ListStyle)
	m.folderTree = buildFolderTree(m.Lists)
}

func (m *Model) revealFolder(folder Folder) {
	if m.folderExpanded == nil {
		m.folderExpanded = map[Folder]bool{}
	}
	for _, ancestor := range ancestorFolders(folder) {
		m.folderExpanded[ancestor] = true
	}
}

func (m *Model) updateFolderTreeNavigation(msg tea.KeyMsg) (tea.Cmd, bool) {
	if m.state != navigatingState || m.pane != folderPane {
		return nil, false
	}

	selectedFolder := m.selectedFolder()
	switch msg.String() {
	case "left", "l":
		if child, ok := m.folderTree.firstChild(selectedFolder); ok {
			m.folderExpanded[selectedFolder] = true
			m.revealFolder(child)
			return m.updateFoldersForSelection(child, true), true
		}

		m.pane = snippetPane
		return m.updateActivePane(msg), true
	case "right":
		if m.folderExpanded[selectedFolder] && m.folderTree.hasChildren(selectedFolder) {
			delete(m.folderExpanded, selectedFolder)
			return m.updateFoldersForSelection(selectedFolder, false), true
		}
		if parent, ok := m.folderTree.parent(selectedFolder); ok {
			return m.updateFoldersForSelection(parent, true), true
		}

		m.pane = snippetPane
		return m.updateActivePane(msg), true
	case "h":
		if m.folderExpanded[selectedFolder] && m.folderTree.hasChildren(selectedFolder) {
			delete(m.folderExpanded, selectedFolder)
			return m.updateFoldersForSelection(selectedFolder, false), true
		}
		if parent, ok := m.folderTree.parent(selectedFolder); ok {
			return m.updateFoldersForSelection(parent, true), true
		}

		return nil, true
	default:
		return nil, false
	}
}

// applyContentView updates the content viewport with the rendered content or
// appropriate hints.
func (m *Model) applyContentView(msg contentRenderedMsg) (tea.Model, tea.Cmd) {
	if msg.showCreateHint {
		m.displayKeyHint([]keyHint{
			{m.keys.NewSnippet, "create a new snippet."},
		})
		return m, nil
	}

	if len(m.List().Items()) <= 0 {
		m.displayKeyHint([]keyHint{
			{m.keys.NewSnippet, "create a new snippet."},
		})
		return m, nil
	}

	if msg.snippet.Path() != m.selectedSnippet().Path() || msg.width != m.contentWidth(m.selectedSnippet()) {
		return m, nil
	}

	if msg.showNoContentHint {
		m.displayKeyHint(m.noContentHints())
		return m, nil
	}

	if msg.err != nil {
		if msg.previewerMissing {
			m.displayError("Install glow to preview Markdown snippets.")
		} else {
			m.displayError(msg.err.Error())
		}
		return m, nil
	}

	if m.contentCache == nil {
		m.contentCache = map[contentCacheKey]contentCacheEntry{}
	}
	m.contentCache[msg.cacheKey] = contentCacheEntry{
		modTimeUnixNano: msg.modTimeUnixNano,
		size:            msg.size,
		rendered:        msg.rendered,
		lineCount:       msg.lineCount,
	}

	m.writeLineNumbers(msg.lineCount)
	m.Code.SetContent(msg.rendered)
	return m, nil
}

type keyHint struct {
	binding key.Binding
	help    string
}

// displayKeyHint updates the content viewport with instructions on the
// relevent key binding that the user should most likely press.
func (m *Model) displayKeyHint(hints []keyHint) {
	m.LineNumbers.SetContent(strings.Repeat("  ~ \n", len(hints)))
	var s strings.Builder
	for _, hint := range hints {
		s.WriteString(
			fmt.Sprintf("%s %s\n",
				m.ContentStyle.EmptyHintKey.Render(hint.binding.Help().Key),
				m.ContentStyle.EmptyHint.Render("• "+hint.help),
			))
	}
	m.Code.SetContent(s.String())
}

func (m *Model) previewWidth() int {
	return m.Code.Width + previewWidthOffset
}

func (m *Model) paneWidths() (int, int) {
	if m.isCollapsedPreview() {
		return 0, 0
	}
	return expandedFolderPaneWidth, expandedSnippetPaneWidth
}

func (m *Model) updatePaneLayout(totalWidth int) {
	if totalWidth <= 0 {
		return
	}

	folderWidth, snippetWidth := m.paneWidths()
	m.setFoldersWidth(folderWidth)
	m.setSnippetsWidth(snippetWidth)

	m.Folders.Title = m.foldersTitle()
	m.LineNumbers.Width = 5
	if m.isCollapsedPreview() {
		m.Code.Width = totalWidth - m.LineNumbers.Width - 2
	} else {
		m.Code.Width = totalWidth - snippetWidth - folderWidth - 20
	}
	if m.Code.Width < minContentPaneWidth {
		m.Code.Width = minContentPaneWidth
	}
}

func (m *Model) setFoldersWidth(width int) {
	m.Folders.SetWidth(width)
	m.FoldersStyle.Base = m.FoldersStyle.Base.Width(width)
	m.FoldersStyle.TitleBar = m.FoldersStyle.TitleBar.Width(maxWidth(width - 2))
	m.Folders.Styles.TitleBar = m.FoldersStyle.TitleBar
	m.Folders.Styles.Title = m.FoldersStyle.Title
}

func (m *Model) setSnippetsWidth(width int) {
	m.ListStyle.Base = m.ListStyle.Base.Width(width)
	m.ListStyle.TitleBar = m.ListStyle.TitleBar.Width(maxWidth(width - 2))
	m.ListStyle.CopiedTitleBar = m.ListStyle.CopiedTitleBar.Width(maxWidth(width - 2))
	m.ListStyle.DeletedTitleBar = m.ListStyle.DeletedTitleBar.Width(maxWidth(width - 2))

	for _, li := range m.Lists {
		li.SetWidth(width)
		li.Styles.Title = m.ListStyle.Title
		li.Styles.TitleBar = m.ListStyle.TitleBar
		li.Styles.StatusBar = lipgloss.NewStyle().Margin(1, 2).Foreground(lipgloss.Color("240")).MaxWidth(maxWidth(width - 2))
		li.Styles.NoItems = lipgloss.NewStyle().Margin(0, 2).Foreground(lipgloss.Color("8")).MaxWidth(maxWidth(width - 2))
	}
}

func (m *Model) foldersTitle() string {
	if m.isCollapsedPreview() {
		return ""
	}
	return "Folders"
}

func (m *Model) snippetsTitle() string {
	if m.isCollapsedPreview() {
		return ""
	}
	return "Snippets"
}

func (m *Model) isCollapsedPreview() bool {
	return m.pane == contentPane && m.state == navigatingState
}

func (m *Model) contentHeader() string {
	if m.state == editingState {
		return lipgloss.JoinHorizontal(lipgloss.Left,
			m.inputs[folderInput].View(),
			m.ContentStyle.Separator.Render("/"),
			m.inputs[nameInput].View(),
			m.ContentStyle.Separator.Render("."),
			m.inputs[languageInput].View(),
		)
	}

	if m.isCollapsedPreview() {
		return m.ContentStyle.Title.Render(m.selectedSnippet().String())
	}

	if len(m.List().Items()) == 0 {
		return m.ContentStyle.Title.Render(string(m.selectedFolder()))
	}

	return lipgloss.JoinHorizontal(lipgloss.Left,
		m.ContentStyle.Title.Render(m.selectedSnippet().Folder),
		m.ContentStyle.Separator.Render("/"),
		m.ContentStyle.Title.Render(m.selectedSnippet().Name),
		m.ContentStyle.Separator.Render("."),
		m.ContentStyle.Title.Render(m.selectedSnippet().Language),
	)
}

// displayError updates the content viewport with the error message provided.
func (m *Model) displayError(error string) {
	m.LineNumbers.SetContent(" ~ ")
	m.LineNumbers.SetYOffset(0)
	m.Code.SetContent(fmt.Sprintf("%s",
		m.ContentStyle.EmptyHint.Render(error),
	))
	m.Code.SetYOffset(0)
}

// writeLineNumbers writes the number of line numbers to the line number
// viewport.
func (m *Model) writeLineNumbers(n int) {
	var lineNumbers strings.Builder
	for i := 1; i < n; i++ {
		lineNumbers.WriteString(fmt.Sprintf("%3d \n", i))
	}
	m.LineNumbers.SetContent(lineNumbers.String() + "  ~ \n")
	m.LineNumbers.SetYOffset(m.Code.YOffset)
}

func (m *Model) updateContentScroll(msg tea.Msg) (tea.Cmd, bool) {
	if m.pane != contentPane || m.state != navigatingState {
		return nil, false
	}

	previousOffset := m.Code.YOffset
	var cmd tea.Cmd
	m.Code, cmd = m.Code.Update(msg)
	if m.Code.YOffset == previousOffset {
		return nil, false
	}

	m.LineNumbers.SetYOffset(m.Code.YOffset)
	return cmd, true
}

const tabSpaces = 4

// updateActivePane updates the currently active pane.
func (m *Model) updateActivePane(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd
	var cmd tea.Cmd
	previousCodeWidth := m.Code.Width
	refreshContent := false
	styles := DefaultStyles(m.config)
	switch m.pane {
	case folderPane:
		previousFolder := m.selectedFolder()
		m.ListStyle = styles.Snippets.Blurred
		m.ContentStyle = styles.Content.Blurred
		m.FoldersStyle = styles.Folders.Focused
		m.Folders, cmd = m.Folders.Update(msg)
		m.updateKeyMap()
		refreshContent = m.selectedFolder() != previousFolder
		cmds = append(cmds, cmd)
	case snippetPane:
		previousSnippet := m.selectedSnippet().Path()
		m.ListStyle = styles.Snippets.Focused
		m.ContentStyle = styles.Content.Blurred
		m.FoldersStyle = styles.Folders.Blurred
		*m.List(), cmd = (*m.List()).Update(msg)
		refreshContent = m.selectedSnippet().Path() != previousSnippet
		cmds = append(cmds, cmd)
	case contentPane:
		m.ListStyle = styles.Snippets.Blurred
		m.ContentStyle = styles.Content.Focused
		m.FoldersStyle = styles.Folders.Blurred
		m.Code, cmd = m.Code.Update(msg)
		cmds = append(cmds, cmd)
		m.LineNumbers, cmd = m.LineNumbers.Update(msg)
		cmds = append(cmds, cmd)
	}
	m.updatePaneLayout(m.width)
	compact := m.isCollapsedPreview()
	m.List().SetDelegate(snippetDelegate{styles: m.ListStyle, state: m.state, compact: compact})
	m.Folders.SetDelegate(folderDelegate{
		styles:   m.FoldersStyle,
		compact:  compact,
		depths:   m.folderTree.depths,
		expanded: m.folderExpanded,
		children: m.folderTree.children,
	})
	m.Folders.Styles.TitleBar = m.FoldersStyle.TitleBar
	m.Folders.Styles.Title = m.FoldersStyle.Title
	if m.Code.Width != previousCodeWidth {
		refreshContent = true
	}
	if refreshContent {
		cmds = append(cmds, m.updateContent())
	}

	return tea.Batch(cmds...)
}

// updateKeyMap disables or enables the keys based on the current state of the
// snippet list.
func (m *Model) updateKeyMap() {
	hasItems := len(m.List().VisibleItems()) > 0
	isFiltering := m.List().FilterState() == list.Filtering
	isEditing := m.state == editingState
	m.keys.DeleteSnippet.SetEnabled(hasItems && !isFiltering && !isEditing)
	m.keys.CopySnippet.SetEnabled(hasItems && !isFiltering && !isEditing)
	m.keys.PasteSnippet.SetEnabled(hasItems && !isFiltering && !isEditing)
	m.keys.EditSnippet.SetEnabled(hasItems && !isFiltering && !isEditing)
	m.keys.NewSnippet.SetEnabled(!isFiltering && !isEditing)
	m.keys.ChangeFolder.SetEnabled(m.pane == folderPane)
}

// selectedSnippet returns the currently selected snippet.
func (m *Model) selectedSnippet() Snippet {
	item := m.List().SelectedItem()
	if item == nil {
		return defaultSnippet
	}
	return item.(Snippet)
}

// selected folder returns the currently selected folder.
func (m *Model) selectedFolder() Folder {
	item := m.Folders.SelectedItem()
	if item == nil {
		return Folder(defaultSnippetFolder)
	}
	return item.(Folder)
}

// List returns the active list.
func (m *Model) List() *list.Model {
	folder := m.selectedFolder()
	if li, ok := m.Lists[folder]; ok {
		return li
	}

	m.Lists[folder] = newList([]list.Item{}, m.height, m.ListStyle)
	return m.Lists[folder]
}

func (m *Model) moveSnippetDown() {
	currentPosition := m.List().Index()
	currentItem := m.List().SelectedItem()
	m.List().InsertItem(currentPosition+2, currentItem)
	m.List().RemoveItem(currentPosition)
	m.List().CursorDown()
}

func (m *Model) moveSnippetUp() {
	currentPosition := m.List().Index()
	currentItem := m.List().SelectedItem()
	m.List().RemoveItem(currentPosition)
	m.List().InsertItem(currentPosition-1, currentItem)
	m.List().CursorUp()
}

// createNewSnippet creates a new snippet file and adds it to the the list.
func (m *Model) createNewSnippetFile() tea.Cmd {
	return func() tea.Msg {
		folder := defaultSnippetFolder
		folderItem := m.Folders.SelectedItem()
		if folderItem != nil && folderItem.FilterValue() != "" {
			folder = folderItem.FilterValue()
		}

		file := fmt.Sprintf("snippet-%d.%s", rand.Intn(1000000), m.config.DefaultLanguage)

		newSnippet := Snippet{
			Name:     defaultSnippetName,
			Date:     time.Now(),
			File:     file,
			Language: m.config.DefaultLanguage,
			Tags:     []string{},
			Folder:   folder,
		}

		path, err := snippetStoragePath(m.config.Home, newSnippet)
		if err != nil {
			return changeStateMsg{navigatingState}
		}
		_ = os.MkdirAll(filepath.Dir(path), os.ModePerm)
		_, _ = os.Create(path)

		m.List().InsertItem(m.List().Index(), newSnippet)
		return changeStateMsg{navigatingState}
	}
}

// View returns the view string for the application model.
func (m *Model) View() string {
	if m.state == quittingState {
		return ""
	}

	var (
		titleBar      = m.ListStyle.TitleBar.Render(m.snippetsTitle())
		contentHeader = m.contentHeader()
	)

	if m.state == copyingState {
		titleBar = m.ListStyle.CopiedTitleBar.Render("Copied Snippet!")
	} else if m.state == deletingState {
		titleBar = m.ListStyle.DeletedTitleBar.Render("Delete Snippet? (y/N)")
	} else if m.List().SettingFilter() {
		titleBar = m.ListStyle.TitleBar.Render(m.List().FilterInput.View())
	}

	contentBody := lipgloss.JoinHorizontal(lipgloss.Left,
		m.ContentStyle.LineNumber.Render(m.LineNumbers.View()),
		m.ContentStyle.Base.Render(strings.ReplaceAll(m.Code.View(), "\t", strings.Repeat(" ", tabSpaces))),
	)

	if m.isCollapsedPreview() {
		return lipgloss.JoinVertical(
			lipgloss.Top,
			contentHeader,
			contentBody,
			marginStyle.Render(m.help.View(m.keys)),
		)
	}

	return lipgloss.JoinVertical(
		lipgloss.Top,
		lipgloss.JoinHorizontal(
			lipgloss.Left,
			m.FoldersStyle.Base.Render(m.Folders.View()),
			m.ListStyle.Base.Render(titleBar+m.List().View()),
			lipgloss.JoinVertical(lipgloss.Top,
				contentHeader,
				contentBody,
			),
		),
		marginStyle.Render(m.help.View(m.keys)),
	)
}

func maxWidth(width int) int {
	if width < 0 {
		return 0
	}
	return width
}

func (m *Model) saveState() {
	s := State{
		CurrentFolder: string(m.selectedFolder()),
	}
	if len(m.List().Items()) > 0 {
		s.CurrentSnippet = m.selectedSnippet().File
	}
	for folder, expanded := range m.folderExpanded {
		if expanded {
			s.ExpandedFolders = append(s.ExpandedFolders, string(folder))
		}
	}
	slices.Sort(s.ExpandedFolders)
	err := s.Save()
	if err != nil {
		panic(err.Error())
	}
}
