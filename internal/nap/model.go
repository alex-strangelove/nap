package nap

import (
	"bytes"
	"errors"
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
	expandedFolderPaneWidth   = 32
	expandedSnippetPaneWidth  = 0
	collapsedSnippetPaneWidth = 0
	minContentPaneWidth       = 20
	previewWidthOffset        = 8
)

type pane int

const (
	contentPane pane = iota
	folderPane
)

const snippetPane pane = contentPane

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
	searchingState
)

type input int

const (
	folderInput input = iota
	nameInput
	languageInput
)

type searchMode int

const (
	previewSearchMode searchMode = iota
	metadataSearchMode
	contentSearchMode
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
	searchInput textinput.Model
	// the current active pane of focus.
	pane pane
	// the current state / action of the application.
	state state
	// stying for components
	ListStyle         SnippetsBaseStyle
	FoldersStyle      FoldersBaseStyle
	ContentStyle      ContentBaseStyle
	contentCache      map[contentCacheKey]contentCacheEntry
	searchResults     *list.Model
	searchDocs        []snippetSearchDoc
	searchDirty       bool
	searchRestorePane pane
	searchMode        searchMode
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
	m.ensureSearchUI()
	m.rebuildFolderTree()

	m.Folders.Styles.Title = m.FoldersStyle.Title
	m.Folders.Styles.TitleBar = m.FoldersStyle.TitleBar
	m.updateKeyMap()

	return m.updateContent()
}

func (m *Model) ensureSearchUI() {
	if m.searchResults == nil {
		m.searchResults = newList([]list.Item{}, m.height, DefaultStyles(m.config).Snippets.Focused)
	}
	if m.searchInput.Prompt == "" {
		m.searchInput = newSearchInput()
	}
}

func newSearchInput() textinput.Model {
	input := textinput.New()
	input.Prompt = "Find: "
	input.Placeholder = "current file"
	input.CharLimit = 0
	return input
}

func (m *Model) configureSearchInput() {
	switch m.searchMode {
	case previewSearchMode:
		m.searchInput.Prompt = "Find: "
		m.searchInput.Placeholder = "current file"
	case metadataSearchMode:
		m.searchInput.Prompt = "Files: "
		m.searchInput.Placeholder = "folders and file names"
	case contentSearchMode:
		m.searchInput.Prompt = "Text: "
		m.searchInput.Placeholder = "file contents"
	}
}

func (m *Model) invalidateSearchIndex() {
	m.searchDirty = true
}

func (m *Model) ensureSearchIndex() {
	if !m.searchDirty && len(m.searchDocs) > 0 {
		return
	}
	m.searchDocs = buildSnippetSearchDocs(m.config.Home, m.allSnippets())
	m.searchDirty = false
}

func (m *Model) allSnippets() []Snippet {
	var snippets []Snippet
	for _, li := range m.Lists {
		for _, item := range li.Items() {
			snippet, ok := item.(Snippet)
			if !ok {
				continue
			}
			snippets = append(snippets, snippet)
		}
	}
	sortSnippets(snippets)
	return snippets
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
	return m.updateFoldersForSelection(m.selectedFolderItem(), false)
}

func (m *Model) updateFoldersForSelection(selected list.Item, refreshContent bool) tea.Cmd {
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
		m.syncSelectedTreeSnippet()
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
		m.invalidateSearchIndex()
		return m, m.updateContent()
	case flashcardsFinishedMsg:
		if msg.err != nil {
			m.displayError(flashcardsError(m.config, msg.err))
			return m, nil
		}
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
				m.invalidateSearchIndex()
				setCmd := m.List().SetItem(i, snippet)
				m.pane = contentPane
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
			m.invalidateSearchIndex()
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
			m.pane = contentPane
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
				if _, ok := m.selectedFolderItem().(Snippet); !ok {
					return m, m.deleteSelectedFolder()
				}
				return m, m.deleteSelectedSnippet()
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
		} else if m.state == searchingState {
			if key.Matches(msg, m.keys.Quit) {
				m.saveState()
				m.state = quittingState
				return m, tea.Quit
			}
			if key.Matches(msg, m.keys.ToggleHelp) {
				m.help.ShowAll = !m.help.ShowAll

				var newHeight int
				if m.help.ShowAll {
					newHeight = m.height - 4
				} else {
					newHeight = m.height
				}
				m.Folders.SetHeight(newHeight)
				m.Code.Height = newHeight
				m.LineNumbers.Height = newHeight
				if m.searchResults != nil {
					m.searchResults.SetHeight(newHeight)
				}
				return m, nil
			}
			if key.Matches(msg, m.keys.Cancel) {
				return m, m.exitSearchMode(false)
			}
			if key.Matches(msg, m.keys.SearchFocusLeft) && m.searchMode == contentSearchMode {
				m.pane = folderPane
				m.updateKeyMap()
				return m, m.updateActivePane(msg)
			}
			if key.Matches(msg, m.keys.SearchFocusRight) && m.searchMode == contentSearchMode {
				m.pane = contentPane
				m.updateKeyMap()
				return m, m.updateActivePane(msg)
			}
			if key.Matches(msg, m.keys.SearchEdit) {
				return m, m.editSearchSelection()
			}
			if msg.String() == "enter" {
				return m, m.exitSearchMode(true)
			}

			searchResultNavigation := false
			if key.Matches(msg, m.keys.SearchPrevious) {
				searchResultNavigation = true
				if m.searchMode == previewSearchMode {
					msg = tea.KeyMsg{Type: tea.KeyUp}
				} else {
					msg = tea.KeyMsg{Type: tea.KeyUp}
				}
			}
			if key.Matches(msg, m.keys.SearchNext) {
				searchResultNavigation = true
				if m.searchMode == previewSearchMode {
					msg = tea.KeyMsg{Type: tea.KeyDown}
				} else {
					msg = tea.KeyMsg{Type: tea.KeyDown}
				}
			}

			if !searchResultNavigation {
				if cmd, ok := m.updateSearchContentScroll(msg); ok {
					return m, cmd
				}
			}

			if m.searchMode != previewSearchMode && m.pane == folderPane && (msg.String() == "up" || msg.String() == "down" || msg.String() == "pgup" || msg.String() == "pgdown" || msg.String() == "home" || msg.String() == "end") {
				previous := m.selectedSnippet().Path()
				var cmd tea.Cmd
				*m.searchResults, cmd = m.searchResults.Update(msg)
				if selected, ok := m.selectedSearchSnippet(); ok && selected.Path() != previous {
					return m, tea.Batch(cmd, m.updateContent())
				}
				return m, cmd
			}

			previousQuery := m.searchInput.Value()
			var cmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(msg)
			if m.searchInput.Value() != previousQuery {
				return m, tea.Batch(cmd, m.refreshSearchResults())
			}
			return m, cmd
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
		case key.Matches(msg, m.keys.CreateFlashcards):
			m.state = creatingState
			return m, m.createFlashcardDeck()
		case key.Matches(msg, m.keys.NewFolder):
			if m.pane != folderPane {
				break
			}
			m.state = creatingState
			return m, m.createNewFolder()
		case key.Matches(msg, m.keys.NewRootFolder):
			if m.pane != folderPane {
				break
			}
			m.state = creatingState
			return m, m.createNewRootFolder()
		case key.Matches(msg, m.keys.PasteSnippet):
			return m, changeState(pastingState)
		case key.Matches(msg, m.keys.RenameSnippet):
			m.activeInput = nameInput
			return m, changeState(editingState)
		case key.Matches(msg, m.keys.ChangeFolder):
			m.pane = contentPane
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
			if m.searchResults != nil {
				m.searchResults.SetHeight(newHeight)
			}
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
			if _, ok := m.selectedFolderItem().(Snippet); !ok {
				break
			}
			return m, changeState(deletingState)
		case key.Matches(msg, m.keys.DeleteFolder):
			if m.pane != folderPane {
				break
			}
			if _, ok := m.selectedFolderItem().(Folder); !ok {
				break
			}
			m.pane = folderPane
			m.updateActivePane(msg)
			m.Folders.Title = "Delete folder? (y/N)"
			return m, changeState(deletingState)
		case key.Matches(msg, m.keys.EditSnippet):
			return m, m.editSnippet()
		case key.Matches(msg, m.keys.ReviewFlashcards):
			return m, m.reviewFlashcards()
		case key.Matches(msg, m.keys.SearchPreview):
			return m, m.enterSearchMode(previewSearchMode, false)
		case key.Matches(msg, m.keys.SearchMetadata):
			return m, m.enterSearchMode(metadataSearchMode, false)
		case key.Matches(msg, m.keys.SearchContents):
			return m, m.enterSearchMode(contentSearchMode, false)
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
		m.pane = contentPane
	}
}

// previousPane sets the previous pane to be active.
func (m *Model) previousPane() {
	switch m.pane {
	case contentPane:
		m.pane = folderPane
	}
}

// editSnippet opens the editor with the selected snippet file path.
func (m *Model) editSnippet() tea.Cmd {
	return tea.ExecProcess(editorCmd(m.selectedSnippetFilePath()), func(err error) tea.Msg {
		return refreshContentMsg{}
	})
}

func (m *Model) editSearchSelection() tea.Cmd {
	path := m.selectedSnippetFilePath()
	if path == "" {
		return nil
	}

	line, column := 0, 0
	query := strings.TrimSpace(m.searchInput.Value())
	if (m.searchMode == previewSearchMode || m.searchMode == contentSearchMode) && query != "" {
		content, err := os.ReadFile(path)
		if err == nil {
			if loc, ok := searchQueryLocation(string(content), query); ok {
				line, column = loc.line, loc.column
			}
		}
	}

	return tea.ExecProcess(searchEditorCmd(path, line, column), func(err error) tea.Msg {
		return refreshContentMsg{}
	})
}

func (m *Model) noContentHints() []keyHint {
	hints := []keyHint{
		{m.keys.EditSnippet, "edit contents"},
		{m.keys.PasteSnippet, "paste clipboard"},
		{m.keys.RenameSnippet, "rename"},
		{m.keys.SetFolder, "set folder"},
		{m.keys.SetLanguage, "set language"},
	}
	if m.config.FlashcardsEnabled {
		hints = append(hints, keyHint{m.keys.ReviewFlashcards, "review cards for this folder"})
		if m.keys.CreateFlashcards.Enabled() {
			hints = append(hints, keyHint{m.keys.CreateFlashcards, "create 00-cards from template"})
		}
	}
	return hints
}

func (m *Model) createHints() []keyHint {
	hints := []keyHint{
		{m.keys.NewSnippet, "create a new snippet."},
	}
	if m.keys.CreateFlashcards.Enabled() {
		hints = append(hints, keyHint{m.keys.CreateFlashcards, "create 00-cards from template."})
	}
	if m.keys.ReviewFlashcards.Enabled() {
		hints = append(hints, keyHint{m.keys.ReviewFlashcards, "review cards for this folder."})
	}
	return hints
}

func (m *Model) folderDeletionTarget() (Folder, bool) {
	folder, ok := m.selectedFolderItem().(Folder)
	return folder, ok
}

// updateFolderView updates the folders list to display the current folders.
func (m *Model) updateFoldersView(selectedItem list.Item, refreshContent bool) tea.Msg {
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

	for _, li := range m.Lists {
		sortSnippetList(li)
	}

	m.rebuildFolderTree()
	if selectedItem == nil {
		selectedItem = m.selectedFolderItem()
	}
	m.revealFolder(treeItemFolder(selectedItem))
	folderItems := m.folderTree.visibleItems(m.folderExpanded)
	selectedFolderIndex := visibleFolderIndex(folderItems, selectedItem, m.folderTree.parents)

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

	selectedItem := m.selectedFolderItem()
	selectedFolder := m.selectedFolder()
	switch msg.String() {
	case "left", "l":
		if snippet, ok := selectedItem.(Snippet); ok {
			m.selectSnippetInFolder(Folder(snippet.Folder), snippet)
			m.pane = contentPane
			return m.updateActivePane(msg), true
		}

		if !m.folderExpanded[selectedFolder] && m.folderTree.hasChildren(selectedFolder) {
			m.folderExpanded[selectedFolder] = true
			return m.updateFoldersForSelection(selectedFolder, false), true
		}

		if child, ok := m.folderTree.firstItem(selectedFolder); ok {
			if folder, ok := child.(Folder); ok {
				m.revealFolder(folder)
			}
			return m.updateFoldersForSelection(child, true), true
		}

		m.pane = contentPane
		return m.updateActivePane(msg), true
	case "right":
		if _, ok := selectedItem.(Snippet); ok {
			return m.updateFoldersForSelection(selectedFolder, true), true
		}
		if m.folderExpanded[selectedFolder] && m.folderTree.hasChildren(selectedFolder) {
			delete(m.folderExpanded, selectedFolder)
			return m.updateFoldersForSelection(selectedFolder, false), true
		}
		if parent, ok := m.folderTree.parent(selectedFolder); ok {
			return m.updateFoldersForSelection(parent, true), true
		}

		m.pane = contentPane
		return m.updateActivePane(msg), true
	case "h":
		if _, ok := selectedItem.(Snippet); ok {
			return m.updateFoldersForSelection(selectedFolder, true), true
		}
		if m.folderExpanded[selectedFolder] && m.folderTree.hasChildren(selectedFolder) {
			delete(m.folderExpanded, selectedFolder)
			return m.updateFoldersForSelection(selectedFolder, false), true
		}
		if parent, ok := m.folderTree.parent(selectedFolder); ok {
			return m.updateFoldersForSelection(parent, true), true
		}

		return nil, true
	case "f":
		if !m.keys.CreateFlashcards.Enabled() {
			return nil, false
		}
		m.state = creatingState
		return m.createFlashcardDeck(), true
	case "F":
		if !m.keys.ReviewFlashcards.Enabled() {
			return nil, false
		}
		return m.reviewFlashcards(), true
	default:
		return nil, false
	}
}

// applyContentView updates the content viewport with the rendered content or
// appropriate hints.
func (m *Model) applyContentView(msg contentRenderedMsg) (tea.Model, tea.Cmd) {
	if msg.showCreateHint {
		m.displayKeyHint(m.createHints())
		return m, nil
	}

	if m.state != searchingState && len(m.List().Items()) <= 0 {
		m.displayKeyHint(m.createHints())
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
	m.Code.SetContent(m.previewContent(msg.rendered))
	return m, nil
}

func (m *Model) previewContent(rendered string) string {
	query := strings.TrimSpace(m.searchInput.Value())
	if !m.shouldHighlightPreview(query) || !m.selectedSnippetContainsQuery(query) {
		return rendered
	}
	return highlightPreviewMatches(rendered, query, m.config)
}

func (m *Model) shouldHighlightPreview(query string) bool {
	if m.state != searchingState || query == "" {
		return false
	}
	return m.searchMode == previewSearchMode || m.searchMode == contentSearchMode
}

func (m *Model) selectedSnippetContainsQuery(query string) bool {
	path := m.selectedSnippetFilePath()
	if path == "" {
		return false
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	return strings.Contains(strings.ToLower(string(content)), strings.ToLower(query))
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
	return expandedFolderPaneWidth, 0
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
	m.setSearchWidth(folderWidth, m.Code.Width)
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

func (m *Model) setSearchWidth(folderWidth, contentWidth int) {
	if m.searchInput.Prompt == "" {
		return
	}
	width := folderWidth
	if m.searchMode == previewSearchMode {
		width = contentWidth
	}
	m.searchInput.Width = maxWidth(width - len(m.searchInput.Prompt) - 2)
	if m.searchResults == nil {
		return
	}
	m.searchResults.SetWidth(folderWidth)
	m.searchResults.SetHeight(m.height)
}

func (m *Model) foldersTitle() string {
	if m.isCollapsedPreview() {
		return ""
	}
	return "Folders"
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

	if m.state == deletingState {
		if snippet, ok := m.selectedFolderItem().(Snippet); ok {
			return m.ContentStyle.Title.Render(fmt.Sprintf("Delete %s? (y/N)", snippet.Name))
		}
		if folder, ok := m.folderDeletionTarget(); ok {
			return m.ContentStyle.Title.Render(fmt.Sprintf("Delete %s? (y/N)", folderLabel(folder)))
		}
	}

	if m.state == searchingState && m.searchMode == previewSearchMode {
		return lipgloss.JoinHorizontal(
			lipgloss.Left,
			m.ContentStyle.Title.Render("Find in file"),
			m.ContentStyle.Separator.Render(" "),
			m.searchInput.View(),
		)
	}

	if selected, ok := m.selectedSearchSnippet(); ok {
		if m.isCollapsedPreview() {
			return m.ContentStyle.Title.Render(selected.String())
		}
		return lipgloss.JoinHorizontal(lipgloss.Left,
			m.ContentStyle.Title.Render(selected.Folder),
			m.ContentStyle.Separator.Render("/"),
			m.ContentStyle.Title.Render(selected.Name),
			m.ContentStyle.Separator.Render("."),
			m.ContentStyle.Title.Render(selected.Language),
		)
	}

	if selected, ok := m.selectedFolderItem().(Snippet); ok {
		if m.isCollapsedPreview() {
			return m.ContentStyle.Title.Render(selected.String())
		}
		return lipgloss.JoinHorizontal(lipgloss.Left,
			m.ContentStyle.Title.Render(selected.Folder),
			m.ContentStyle.Separator.Render("/"),
			m.ContentStyle.Title.Render(selected.Name),
			m.ContentStyle.Separator.Render("."),
			m.ContentStyle.Title.Render(selected.Language),
		)
	}

	if len(m.List().Items()) == 0 {
		return m.ContentStyle.Title.Render(string(m.selectedFolder()))
	}
	return m.ContentStyle.Title.Render(string(m.selectedFolder()))
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

func (m *Model) updateSearchContentScroll(msg tea.Msg) (tea.Cmd, bool) {
	if m.pane != contentPane || m.state != searchingState {
		return nil, false
	}
	if m.searchMode != previewSearchMode && m.searchMode != contentSearchMode {
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
		previousItem := m.selectedFolderItem()
		m.ListStyle = styles.Snippets.Blurred
		m.ContentStyle = styles.Content.Blurred
		m.FoldersStyle = styles.Folders.Focused
		m.Folders, cmd = m.Folders.Update(msg)
		m.syncSelectedTreeSnippet()
		m.updateKeyMap()
		refreshContent = !sameTreeItem(previousItem, m.selectedFolderItem())
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
	if m.searchResults != nil {
		searchStyles := DefaultStyles(m.config).Snippets.Focused
		m.searchResults.SetDelegate(snippetDelegate{styles: searchStyles, state: navigatingState, compact: compact})
	}
	m.Folders.SetDelegate(folderDelegate{
		styles:   m.FoldersStyle,
		compact:  compact,
		depths:   m.folderTree.depths,
		expanded: m.folderExpanded,
		children: m.folderTree.children,
		snippets: m.folderTree.snippets,
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
	selectedList, hasSelectedList := m.Lists[m.selectedFolder()]
	hasFolderItems := len(m.Folders.VisibleItems()) > 0
	isFiltering := hasSelectedList && selectedList.FilterState() == list.Filtering
	isEditing := m.state == editingState
	isSearching := m.state == searchingState
	deckCount := len(m.flashcardDecks(m.selectedFolder()))
	flashcardsReady := m.config.FlashcardsEnabled && !isFiltering && !isEditing && !isSearching

	m.keys.flashcardsEnabled = m.config.FlashcardsEnabled
	_, snippetSelected := m.selectedFolderItem().(Snippet)
	m.keys.DeleteSnippet.SetEnabled(snippetSelected && !isFiltering && !isEditing && !isSearching)
	_, folderSelected := m.selectedFolderItem().(Folder)
	m.keys.DeleteFolder.SetEnabled(m.pane == folderPane && folderSelected && hasFolderItems && !isEditing && !isSearching)
	m.keys.CopySnippet.SetEnabled(snippetSelected && !isFiltering && !isEditing && !isSearching)
	m.keys.PasteSnippet.SetEnabled(snippetSelected && !isFiltering && !isEditing && !isSearching)
	m.keys.EditSnippet.SetEnabled(snippetSelected && !isFiltering && !isEditing && !isSearching)
	m.keys.NewSnippet.SetEnabled(!isFiltering && !isEditing && !isSearching)
	m.keys.NewFolder.SetEnabled(m.pane == folderPane && !isEditing && !isSearching)
	m.keys.NewRootFolder.SetEnabled(m.pane == folderPane && !isEditing && !isSearching)
	m.keys.ChangeFolder.SetEnabled(m.pane == folderPane && !isSearching)
	m.keys.CreateFlashcards.SetEnabled(flashcardsReady && deckCount == 0)
	m.keys.ReviewFlashcards.SetEnabled(flashcardsReady && deckCount == 1)
	m.keys.SearchPreview.SetEnabled(!isEditing && !isSearching)
	m.keys.SearchMetadata.SetEnabled(!isEditing && !isSearching)
	m.keys.SearchContents.SetEnabled(!isEditing && !isSearching)
	m.keys.SearchNext.SetEnabled(isSearching)
	m.keys.SearchPrevious.SetEnabled(isSearching)
	m.keys.SearchEdit.SetEnabled(isSearching)
	m.keys.SearchFocusLeft.SetEnabled(isSearching && m.searchMode == contentSearchMode)
	m.keys.SearchFocusRight.SetEnabled(isSearching && m.searchMode == contentSearchMode)
}

func (m *Model) flashcardDecks(folder Folder) []Snippet {
	li, ok := m.Lists[folder]
	if !ok || li == nil {
		return nil
	}

	return flashcardDecks(li.Items())
}

func (m *Model) currentFlashcardDeck() (Snippet, error) {
	decks := m.flashcardDecks(m.selectedFolder())
	switch len(decks) {
	case 0:
		return Snippet{}, errFlashcardDeckMissing
	case 1:
		return decks[0], nil
	default:
		return Snippet{}, errFlashcardDeckAmbiguous
	}
}

// selectedSnippet returns the currently selected snippet.
func (m *Model) selectedSnippet() Snippet {
	if snippet, ok := m.selectedSearchSnippet(); ok {
		return snippet
	}
	if snippet, ok := m.selectedFolderItem().(Snippet); ok {
		return snippet
	}
	item := m.List().SelectedItem()
	if item == nil {
		return defaultSnippet
	}
	return item.(Snippet)
}

// selected folder returns the currently selected folder.
func (m *Model) selectedFolder() Folder {
	item := m.selectedFolderItem()
	if item == nil {
		return Folder(defaultSnippetFolder)
	}
	return treeItemFolder(item)
}

func (m *Model) selectedFolderItem() list.Item {
	return m.Folders.SelectedItem()
}

func (m *Model) selectedSearchSnippet() (Snippet, bool) {
	if m.state != searchingState || m.searchMode == previewSearchMode || m.searchResults == nil {
		return Snippet{}, false
	}
	item := m.searchResults.SelectedItem()
	snippet, ok := item.(Snippet)
	return snippet, ok
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

func (m *Model) refreshSearchResults() tea.Cmd {
	m.ensureSearchUI()
	if m.searchMode == previewSearchMode {
		return m.updateContent()
	}
	m.ensureSearchIndex()

	currentPath := ""
	if snippet, ok := m.selectedSearchSnippet(); ok {
		currentPath = snippet.Path()
	} else if snippet, ok := m.selectedFolderItem().(Snippet); ok {
		currentPath = snippet.Path()
	}

	var results []Snippet
	switch m.searchMode {
	case metadataSearchMode:
		results = searchSnippetMetadataDocs(m.searchDocs, m.searchInput.Value())
	case contentSearchMode:
		results = searchSnippetContentDocs(m.searchDocs, m.searchInput.Value())
	default:
		results = searchSnippetDocs(m.searchDocs, m.searchInput.Value())
	}
	items := make([]list.Item, 0, len(results))
	selectedIndex := 0
	for idx, snippet := range results {
		items = append(items, snippet)
		if snippet.Path() == currentPath {
			selectedIndex = idx
		}
	}

	setItemsCmd := m.searchResults.SetItems(items)
	if len(items) == 0 {
		m.searchResults.Select(0)
		return tea.Batch(setItemsCmd, m.updateContent())
	}

	m.searchResults.Select(selectedIndex)
	return tea.Batch(setItemsCmd, m.updateContent())
}

func (m *Model) enterSearchMode(mode searchMode, preserveQuery bool) tea.Cmd {
	m.ensureSearchUI()
	if m.state != searchingState {
		m.searchRestorePane = m.pane
	}
	m.state = searchingState
	m.searchMode = mode
	m.configureSearchInput()
	if mode == previewSearchMode {
		m.pane = contentPane
	} else {
		m.pane = folderPane
	}
	if !preserveQuery {
		m.searchInput.SetValue("")
	}
	m.searchInput.CursorEnd()
	m.updateKeyMap()
	return tea.Batch(m.updateActivePane(changeStateMsg{newState: searchingState}), m.searchInput.Focus(), m.refreshSearchResults())
}

func (m *Model) exitSearchMode(applySelection bool) tea.Cmd {
	selected, hasSelection := m.selectedSearchSnippet()
	m.state = navigatingState
	m.pane = m.searchRestorePane
	m.searchInput.Blur()
	m.updateKeyMap()
	if applySelection && hasSelection {
		return m.updateFoldersForSelection(selected, true)
	}
	return m.updateContent()
}

// createNewSnippet creates a new snippet file and adds it to the the list.
func (m *Model) createNewSnippetFile() tea.Cmd {
	return func() tea.Msg {
		folder := string(m.selectedFolder())
		if folder == "" {
			folder = defaultSnippetFolder
		}

		li, ok := m.Lists[Folder(folder)]
		if !ok {
			li = newList([]list.Item{}, m.height, m.ListStyle)
			m.Lists[Folder(folder)] = li
		}

		name := nextIndexedMixedName(li.Items(), m.folderTree.children[Folder(folder)], defaultIndexedSnippetStem)
		file := fmt.Sprintf("%s.%s", name, m.config.DefaultLanguage)

		newSnippet := Snippet{
			Name:     name,
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

		li.InsertItem(len(li.Items()), newSnippet)
		sortSnippetList(li)
		m.selectSnippetInFolder(Folder(folder), newSnippet)
		m.invalidateSearchIndex()
		m.state = navigatingState
		m.updateKeyMap()
		return m.updateFoldersForSelection(Folder(folder), true)()
	}
}

func (m *Model) createFlashcardDeck() tea.Cmd {
	return func() tea.Msg {
		deck, err := m.ensureFlashcardDeck()
		if err != nil {
			m.state = navigatingState
			m.updateKeyMap()
			if errors.Is(err, errFlashcardDeckAmbiguous) {
				m.displayError("Multiple flashcard decks found in this folder.")
				return m.updateFoldersForSelection(m.selectedFolderItem(), false)()
			}
			m.displayError(err.Error())
			return m.updateFoldersForSelection(m.selectedFolderItem(), false)()
		}
		m.state = navigatingState
		m.updateKeyMap()
		return m.updateFoldersForSelection(deck, true)()
	}
}

func (m *Model) ensureFlashcardDeck() (Snippet, error) {
	folder := m.selectedFolder()
	decks := m.flashcardDecks(folder)
	switch len(decks) {
	case 0:
	case 1:
		return decks[0], nil
	default:
		return Snippet{}, errFlashcardDeckAmbiguous
	}

	deck := Snippet{
		Name:     defaultFlashcardDeckStem,
		Date:     time.Now(),
		File:     defaultFlashcardDeckStem + defaultFlashcardExtension,
		Language: defaultFlashcardLanguage,
		Tags:     []string{},
		Folder:   string(folder),
	}

	path, err := snippetStoragePath(m.config.Home, deck)
	if err != nil {
		return Snippet{}, err
	}

	_ = os.MkdirAll(filepath.Dir(path), os.ModePerm)
	if _, err := os.Stat(path); err == nil {
		m.insertFlashcardDeck(folder, deck)
		return deck, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return Snippet{}, err
	}

	if err := os.WriteFile(path, []byte(defaultFlashcardDeckContent()), 0o644); err != nil {
		return Snippet{}, err
	}

	m.insertFlashcardDeck(folder, deck)
	m.invalidateSearchIndex()
	return deck, nil
}

func (m *Model) insertFlashcardDeck(folder Folder, deck Snippet) {
	li, ok := m.Lists[folder]
	if !ok {
		li = newList([]list.Item{}, m.height, m.ListStyle)
		m.Lists[folder] = li
	}
	for _, item := range li.Items() {
		snippet, ok := item.(Snippet)
		if ok && snippet.File == deck.File {
			m.selectSnippetInFolder(folder, snippet)
			return
		}
	}
	li.InsertItem(len(li.Items()), deck)
	sortSnippetList(li)
	m.selectSnippetInFolder(folder, deck)
}

func (m *Model) createNewFolder() tea.Cmd {
	return m.createNewFolderAt(m.selectedFolder())
}

func (m *Model) createNewRootFolder() tea.Cmd {
	return m.createNewFolderAt("")
}

func (m *Model) reviewFlashcards() tea.Cmd {
	deck, err := m.currentFlashcardDeck()
	if err != nil {
		if errors.Is(err, errFlashcardDeckMissing) {
			m.displayError("Flashcard deck not found in this folder.")
			return nil
		}
		m.displayError("Multiple flashcard decks found in this folder.")
		return nil
	}

	path, err := snippetStoragePath(m.config.Home, deck)
	if err != nil {
		m.displayError(err.Error())
		return nil
	}

	cmd, err := flashcardsCmd(m.config, path)
	if err != nil {
		m.displayError(flashcardsError(m.config, err))
		return nil
	}

	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return flashcardsFinishedMsg{err: err}
	})
}

func (m *Model) createNewFolderAt(parent Folder) tea.Cmd {
	return func() tea.Msg {
		var items []list.Item
		if li, ok := m.Lists[parent]; ok {
			items = li.Items()
		}
		children := m.folderTree.children[parent]
		if parent == "" {
			children = m.folderTree.roots
		}
		name := nextIndexedMixedName(items, children, defaultIndexedFolderStem)
		folder := Folder(filepath.ToSlash(filepath.Join(string(parent), name)))
		snippet := Snippet{
			Name:     defaultIndexSnippetName,
			Date:     time.Now(),
			File:     defaultIndexSnippetName + "." + defaultIndexLanguage,
			Language: defaultIndexLanguage,
			Tags:     []string{},
			Folder:   string(folder),
		}

		path, err := snippetStoragePath(m.config.Home, snippet)
		if err != nil {
			m.state = navigatingState
			m.updateKeyMap()
			return m.updateFoldersForSelection(m.selectedFolderItem(), true)()
		}
		_ = os.MkdirAll(filepath.Dir(path), os.ModePerm)
		_ = os.WriteFile(path, []byte(defaultFolderIndexContent(folder)), 0o644)

		m.Lists[folder] = newList([]list.Item{snippet}, m.height, m.ListStyle)
		if m.folderExpanded == nil {
			m.folderExpanded = map[Folder]bool{}
		}
		m.folderExpanded[folder] = true
		m.invalidateSearchIndex()
		m.state = navigatingState
		m.updateKeyMap()
		return m.updateFoldersForSelection(snippet, true)()
	}
}

func (m *Model) deleteSelectedSnippet() tea.Cmd {
	return func() tea.Msg {
		snippet, ok := m.selectedFolderItem().(Snippet)
		if !ok {
			return m.updateFoldersForSelection(m.selectedFolderItem(), false)()
		}
		_ = os.Remove(m.selectedSnippetFilePath())
		for idx, item := range m.List().Items() {
			candidate, ok := item.(Snippet)
			if ok && candidate.Path() == snippet.Path() {
				m.List().RemoveItem(idx)
				break
			}
		}
		m.invalidateSearchIndex()
		m.state = navigatingState
		m.updateKeyMap()
		return m.updateFoldersForSelection(m.selectedFolder(), true)()
	}
}

func (m *Model) deleteSelectedFolder() tea.Cmd {
	return func() tea.Msg {
		target := m.selectedFolder()
		var fallback list.Item
		if parent, ok := parentFolder(target); ok {
			fallback = parent
		}

		path, err := resolveHomePath(m.config.Home, string(target))
		if err != nil {
			m.state = navigatingState
			m.updateKeyMap()
			m.displayError(err.Error())
			return m.updateFoldersForSelection(m.selectedFolderItem(), false)()
		}

		_ = os.RemoveAll(path)
		for folder := range m.Lists {
			if !isSameOrDescendantFolder(target, folder) {
				continue
			}
			delete(m.Lists, folder)
			delete(m.folderExpanded, folder)
		}
		delete(m.folderExpanded, target)
		m.invalidateSearchIndex()
		m.state = navigatingState
		m.updateKeyMap()
		return m.updateFoldersForSelection(fallback, true)()
	}
}

func (m *Model) syncSelectedTreeSnippet() {
	snippet, ok := m.selectedFolderItem().(Snippet)
	if !ok {
		return
	}
	m.selectSnippetInFolder(Folder(snippet.Folder), snippet)
}

func (m *Model) selectSnippetInFolder(folder Folder, target Snippet) {
	li, exists := m.Lists[folder]
	if !exists {
		return
	}

	for idx, item := range li.Items() {
		snippet, ok := item.(Snippet)
		if ok && snippet.Path() == target.Path() {
			li.Select(idx)
			return
		}
	}
}

func sameTreeItem(left, right list.Item) bool {
	switch l := left.(type) {
	case Folder:
		r, ok := right.(Folder)
		return ok && l == r
	case Snippet:
		r, ok := right.(Snippet)
		return ok && l.Path() == r.Path()
	default:
		return left == nil && right == nil
	}
}

// View returns the view string for the application model.
func (m *Model) View() string {
	if m.state == quittingState {
		return ""
	}

	var (
		contentHeader = m.contentHeader()
	)

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
			m.FoldersStyle.Base.Render(m.leftPaneView()),
			lipgloss.JoinVertical(lipgloss.Top,
				contentHeader,
				contentBody,
			),
		),
		marginStyle.Render(m.help.View(m.keys)),
	)
}

func (m *Model) leftPaneView() string {
	if m.state != searchingState || m.searchMode == previewSearchMode || m.searchResults == nil {
		return m.Folders.View()
	}
	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.searchInput.View(),
		m.searchResults.View(),
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
	if _, ok := m.selectedFolderItem().(Snippet); ok && len(m.List().Items()) > 0 {
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
