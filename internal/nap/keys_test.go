package nap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func TestPaneNavigationUsesHLKeys(t *testing.T) {
	m := newTestModel()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if m.pane != contentPane {
		t.Fatalf("pane after pressing l is incorrect: got %v want %v", m.pane, contentPane)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if m.pane != snippetPane {
		t.Fatalf("pane after pressing h is incorrect: got %v want %v", m.pane, snippetPane)
	}
}

func TestPaneNavigationStopsAtEdges(t *testing.T) {
	m := newTestModel()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if m.pane != folderPane {
		t.Fatalf("pane after pressing h from snippets is incorrect: got %v want %v", m.pane, folderPane)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if m.pane != folderPane {
		t.Fatalf("pane after pressing h at left edge is incorrect: got %v want %v", m.pane, folderPane)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if m.pane != contentPane {
		t.Fatalf("pane after pressing l at right edge is incorrect: got %v want %v", m.pane, contentPane)
	}
}

func TestPreviewPaneCollapsesLeftColumns(t *testing.T) {
	m := newTestModel()

	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if m.Folders.Width() != expandedFolderPaneWidth {
		t.Fatalf("folder width before preview is incorrect: got %d want %d", m.Folders.Width(), expandedFolderPaneWidth)
	}
	if m.List().Width() != expandedSnippetPaneWidth {
		t.Fatalf("snippet width before preview is incorrect: got %d want %d", m.List().Width(), expandedSnippetPaneWidth)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if m.pane != contentPane {
		t.Fatalf("pane after pressing l is incorrect: got %v want %v", m.pane, contentPane)
	}
	if m.Folders.Width() != 0 {
		t.Fatalf("folder width in preview is incorrect: got %d want %d", m.Folders.Width(), 0)
	}
	if m.List().Width() != 0 {
		t.Fatalf("snippet width in preview is incorrect: got %d want %d", m.List().Width(), 0)
	}
	if m.Folders.Title != "" {
		t.Fatalf("folder title in preview is incorrect: got %q want empty", m.Folders.Title)
	}
	view := m.View()
	if !strings.Contains(view, defaultSnippet.String()) {
		t.Fatalf("preview header does not contain snippet path %q", defaultSnippet.String())
	}
	if strings.Contains(view, "Folders") || strings.Contains(view, "Snippets") {
		t.Fatalf("preview view still contains pane labels: %q", view)
	}
}

func TestEditingMetadataKeepsExpandedLeftColumns(t *testing.T) {
	m := newTestModel()

	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m.Update(changeStateMsg{newState: editingState})

	if m.pane != contentPane {
		t.Fatalf("pane in editing state is incorrect: got %v want %v", m.pane, contentPane)
	}
	if m.Folders.Width() != expandedFolderPaneWidth {
		t.Fatalf("folder width while editing is incorrect: got %d want %d", m.Folders.Width(), expandedFolderPaneWidth)
	}
	if m.List().Width() != expandedSnippetPaneWidth {
		t.Fatalf("snippet width while editing is incorrect: got %d want %d", m.List().Width(), expandedSnippetPaneWidth)
	}
	if m.Folders.Title != "Folders" {
		t.Fatalf("folder title while editing is incorrect: got %q want %q", m.Folders.Title, "Folders")
	}
}

func newTestModel() *Model {
	config := newConfig()
	styles := DefaultStyles(config)
	snippet := defaultSnippet

	lists := map[Folder]*list.Model{
		Folder(defaultSnippetFolder): newList([]list.Item{snippet}, 20, styles.Snippets.Focused),
	}
	tree := buildFolderTree(lists)
	folderExpanded := map[Folder]bool{}
	folderList := list.New([]list.Item{Folder(defaultSnippetFolder)}, folderDelegate{
		styles:   styles.Folders.Blurred,
		depths:   tree.depths,
		expanded: folderExpanded,
		children: tree.children,
	}, 0, 0)
	folderList.Select(0)

	m := &Model{
		config:         config,
		keys:           DefaultKeyMap,
		help:           help.New(),
		Lists:          lists,
		Folders:        folderList,
		folderTree:     tree,
		folderExpanded: folderExpanded,
		Code:           viewport.New(80, 0),
		LineNumbers:    viewport.New(5, 0),
		ListStyle:      styles.Snippets.Focused,
		FoldersStyle:   styles.Folders.Blurred,
		ContentStyle:   styles.Content.Blurred,
		inputs: []textinput.Model{
			newTextInput(defaultSnippetFolder + " "),
			newTextInput(defaultSnippetName + " "),
			newTextInput(config.DefaultLanguage),
		},
		contentCache: map[contentCacheKey]contentCacheEntry{},
	}
	m.updateKeyMap()
	return m
}

func newNestedFolderTestModel() *Model {
	config := newConfig()
	styles := DefaultStyles(config)
	lists := map[Folder]*list.Model{
		Folder("work/backend"): newList([]list.Item{
			Snippet{
				Name:     "handler",
				Folder:   "work/backend",
				File:     "handler.go",
				Language: "go",
			},
		}, 20, styles.Snippets.Focused),
	}
	ensureAncestorLists(lists, 20, styles.Snippets.Focused)
	tree := buildFolderTree(lists)
	folderExpanded := map[Folder]bool{}
	items := tree.visibleItems(folderExpanded)
	folderList := list.New(items, folderDelegate{
		styles:   styles.Folders.Blurred,
		depths:   tree.depths,
		expanded: folderExpanded,
		children: tree.children,
	}, 0, 0)
	folderList.Select(0)

	m := &Model{
		config:         config,
		keys:           DefaultKeyMap,
		help:           help.New(),
		Lists:          lists,
		Folders:        folderList,
		folderTree:     tree,
		folderExpanded: folderExpanded,
		Code:           viewport.New(80, 0),
		LineNumbers:    viewport.New(5, 0),
		ListStyle:      styles.Snippets.Focused,
		FoldersStyle:   styles.Folders.Blurred,
		ContentStyle:   styles.Content.Blurred,
		inputs: []textinput.Model{
			newTextInput(defaultSnippetFolder + " "),
			newTextInput(defaultSnippetName + " "),
			newTextInput(config.DefaultLanguage),
		},
		contentCache: map[contentCacheKey]contentCacheEntry{},
		pane:         folderPane,
	}
	m.updateKeyMap()
	return m
}

func TestSnippetDelegateUpdateDoesNotRefreshContent(t *testing.T) {
	delegate := snippetDelegate{}
	model := list.New([]list.Item{defaultSnippet}, delegate, 0, 0)

	if cmd := delegate.Update(tea.KeyMsg{Type: tea.KeyDown}, &model); cmd != nil {
		t.Fatalf("snippet delegate should not force content refresh commands")
	}
}

func TestUpdateContentUsesCachedRenderForMarkdown(t *testing.T) {
	tmp := t.TempDir()
	content := "# title\n\ncontent"
	snippet := Snippet{
		Name:     "preview",
		Folder:   defaultSnippetFolder,
		File:     "preview.md",
		Language: "md",
	}
	snippetPath := filepath.Join(tmp, snippet.Path())
	if err := os.MkdirAll(filepath.Dir(snippetPath), os.ModePerm); err != nil {
		t.Fatalf("could not create snippet dir: %v", err)
	}
	if err := os.WriteFile(snippetPath, []byte(content), 0o644); err != nil {
		t.Fatalf("could not write snippet: %v", err)
	}

	m := newTestModel()
	m.config.Home = tmp
	m.Lists[Folder(defaultSnippetFolder)].SetItem(0, snippet)
	m.Code.Width = 40

	info, err := os.Stat(snippetPath)
	if err != nil {
		t.Fatalf("could not stat snippet: %v", err)
	}
	width := m.contentWidth(snippet)
	key := m.contentKey(snippet, width)
	m.contentCache[key] = contentCacheEntry{
		modTimeUnixNano: info.ModTime().UnixNano(),
		size:            info.Size(),
		rendered:        "cached preview",
		lineCount:       2,
	}

	cmd := m.updateContent()
	if cmd == nil {
		t.Fatalf("updateContent returned nil")
	}

	msg, ok := cmd().(contentRenderedMsg)
	if !ok {
		t.Fatalf("updateContent returned unexpected message type %T", cmd())
	}
	if msg.rendered != "cached preview" {
		t.Fatalf("cached preview content mismatch: got %q", msg.rendered)
	}
	if msg.width != width {
		t.Fatalf("cached preview width mismatch: got %d want %d", msg.width, width)
	}
}

func TestContentPaneScrollSkipsFullPaneRefresh(t *testing.T) {
	m := newTestModel()
	m.pane = contentPane
	m.state = navigatingState
	m.Code.Height = 3
	m.LineNumbers.Height = 3
	m.Code.SetContent("one\ntwo\nthree\nfour\nfive")
	m.writeLineNumbers(5)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Fatalf("expected no extra command while scrolling content pane")
	}

	got := updated.(*Model)
	if got.Code.YOffset != 1 {
		t.Fatalf("code viewport offset mismatch: got %d want 1", got.Code.YOffset)
	}
	if got.LineNumbers.YOffset != 1 {
		t.Fatalf("line number viewport offset mismatch: got %d want 1", got.LineNumbers.YOffset)
	}
}

func TestCreateNewSnippetFileCreatesNestedFolderPath(t *testing.T) {
	tmp := t.TempDir()
	m := newTestModel()
	nestedFolder := Folder("foo/bar")
	m.config.Home = tmp
	m.Lists[nestedFolder] = newList([]list.Item{}, 20, m.ListStyle)
	m.Folders.SetItems([]list.Item{nestedFolder})
	m.Folders.Select(0)

	cmd := m.createNewSnippetFile()
	if cmd == nil {
		t.Fatalf("createNewSnippetFile returned nil")
	}
	msg := cmd()
	if _, ok := msg.(changeStateMsg); !ok {
		t.Fatalf("unexpected createNewSnippetFile message type %T", msg)
	}

	items := m.Lists[nestedFolder].Items()
	if len(items) != 1 {
		t.Fatalf("expected one snippet in nested folder, got %d", len(items))
	}
	snippet := items[0].(Snippet)
	path, err := snippetStoragePath(tmp, snippet)
	if err != nil {
		t.Fatalf("could not resolve snippet path: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("nested snippet file was not created: %v", err)
	}
}

func TestFolderPaneLeftExpandsAndDescendsTree(t *testing.T) {
	m := newNestedFolderTestModel()

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if cmd == nil {
		t.Fatalf("expected folder tree expansion command")
	}

	updated, cmd = updated.(*Model).Update(cmd())
	got := updated.(*Model)
	if got.selectedFolder() != Folder("work/backend") {
		t.Fatalf("selected folder mismatch: got %q want %q", got.selectedFolder(), Folder("work/backend"))
	}
	if !got.folderExpanded[Folder("work")] {
		t.Fatalf("expected parent folder to be expanded")
	}
	if len(got.Folders.Items()) != 2 {
		t.Fatalf("visible folder count mismatch: got %d want 2", len(got.Folders.Items()))
	}
}

func TestFolderPaneLeftOnLeafMovesToSnippets(t *testing.T) {
	m := newNestedFolderTestModel()
	m.folderExpanded[Folder("work")] = true

	msg := m.updateFoldersView(Folder("work/backend"), false).(updateFoldersMsg)
	m.Folders.SetItems(msg.items)
	m.Folders.Select(msg.selectedFolderIndex)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	got := updated.(*Model)
	if got.pane != snippetPane {
		t.Fatalf("pane mismatch: got %v want %v", got.pane, snippetPane)
	}
	if cmd != nil {
		if _, ok := cmd().(contentRenderedMsg); !ok {
			t.Fatalf("unexpected follow-up message type %T", cmd())
		}
	}
}

func TestFolderPaneLExpandsAndDescendsTree(t *testing.T) {
	m := newNestedFolderTestModel()

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if cmd == nil {
		t.Fatalf("expected folder tree expansion command")
	}

	updated, _ = updated.(*Model).Update(cmd())
	got := updated.(*Model)
	if got.selectedFolder() != Folder("work/backend") {
		t.Fatalf("selected folder mismatch: got %q want %q", got.selectedFolder(), Folder("work/backend"))
	}
	if !got.folderExpanded[Folder("work")] {
		t.Fatalf("expected parent folder to be expanded")
	}
}

func TestFolderPaneHCollapsesToParent(t *testing.T) {
	m := newNestedFolderTestModel()
	m.folderExpanded[Folder("work")] = true

	msg := m.updateFoldersView(Folder("work/backend"), false).(updateFoldersMsg)
	m.Folders.SetItems(msg.items)
	m.Folders.Select(msg.selectedFolderIndex)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if cmd == nil {
		t.Fatalf("expected folder tree collapse command")
	}

	updated, _ = updated.(*Model).Update(cmd())
	got := updated.(*Model)
	if got.selectedFolder() != Folder("work") {
		t.Fatalf("selected folder mismatch: got %q want %q", got.selectedFolder(), Folder("work"))
	}
}

func TestFolderPaneHStopsAtRoot(t *testing.T) {
	m := newTestModel()
	m.pane = folderPane

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if cmd != nil {
		t.Fatalf("expected no command when stopping at root folder")
	}

	got := updated.(*Model)
	if got.pane != folderPane {
		t.Fatalf("pane mismatch: got %v want %v", got.pane, folderPane)
	}
}

func TestContentHeaderUsesSelectedFolderForParentNode(t *testing.T) {
	m := newNestedFolderTestModel()
	m.pane = folderPane

	header := m.contentHeader()
	if !strings.Contains(header, "work") {
		t.Fatalf("content header should include selected folder, got %q", header)
	}
	if strings.Contains(header, defaultSnippet.String()) {
		t.Fatalf("content header should not use default snippet placeholder for empty parent folders")
	}
}
