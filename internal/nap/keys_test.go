package nap

import (
	"bytes"
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

	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.pane != contentPane {
		t.Fatalf("pane after pressing tab is incorrect: got %v want %v", m.pane, contentPane)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.pane != folderPane {
		t.Fatalf("pane after pressing shift+tab is incorrect: got %v want %v", m.pane, folderPane)
	}
}

func TestPaneNavigationStopsAtEdges(t *testing.T) {
	m := newTestModel()

	m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.pane != folderPane {
		t.Fatalf("pane after pressing shift+tab at left edge is incorrect: got %v want %v", m.pane, folderPane)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.pane != contentPane {
		t.Fatalf("pane after pressing tab at right edge is incorrect: got %v want %v", m.pane, contentPane)
	}
}

func TestPreviewPaneCollapsesLeftColumns(t *testing.T) {
	m := newTestModel()

	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if m.Folders.Width() != expandedFolderPaneWidth {
		t.Fatalf("folder width before preview is incorrect: got %d want %d", m.Folders.Width(), expandedFolderPaneWidth)
	}
	if m.List().Width() != 0 {
		t.Fatalf("snippet width before preview is incorrect: got %d want %d", m.List().Width(), 0)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.pane != contentPane {
		t.Fatalf("pane after pressing tab is incorrect: got %v want %v", m.pane, contentPane)
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
	if !strings.Contains(view, defaultSnippetFolder) {
		t.Fatalf("preview header does not contain folder name %q", defaultSnippetFolder)
	}
	if strings.Contains(view, "Folders") {
		t.Fatalf("preview view still contains pane labels: %q", view)
	}
}

func TestShortHelpIncludesDeleteSnippetAndRootFolderHintsWhenDisabled(t *testing.T) {
	m := newTestModel()
	m.help.Width = 200

	if m.keys.DeleteSnippet.Enabled() {
		t.Fatalf("delete snippet should be disabled when a folder is selected")
	}

	view := m.help.ShortHelpView(m.keys.ShortHelp())
	if !strings.Contains(view, "x") || !strings.Contains(view, "del snippet") {
		t.Fatalf("short help should display delete snippet hint, got %q", view)
	}
	if !strings.Contains(view, "O") || !strings.Contains(view, "new root folder") {
		t.Fatalf("short help should display new root folder hint, got %q", view)
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
	if m.List().Width() != 0 {
		t.Fatalf("snippet width while editing is incorrect: got %d want %d", m.List().Width(), 0)
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
		snippets: tree.snippets,
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
		snippets: tree.snippets,
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

func newBoundIndexTestModel() *Model {
	config := newConfig()
	styles := DefaultStyles(config)
	lists := map[Folder]*list.Model{
		Folder("work"): newList([]list.Item{
			Snippet{
				Name:     "01-index",
				Folder:   "work",
				File:     "01-index.md",
				Language: "md",
			},
		}, 20, styles.Snippets.Focused),
		Folder("work/tools"): newList([]list.Item{
			Snippet{
				Name:     "01-index",
				Folder:   "work/tools",
				File:     "01-index.md",
				Language: "md",
			},
		}, 20, styles.Snippets.Focused),
	}
	tree := buildFolderTree(lists)
	folderExpanded := map[Folder]bool{}
	items := tree.visibleItems(folderExpanded)
	folderList := list.New(items, folderDelegate{
		styles:   styles.Folders.Blurred,
		depths:   tree.depths,
		expanded: folderExpanded,
		children: tree.children,
		snippets: tree.snippets,
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

func TestFolderTreeLongLabelsAreEllipsized(t *testing.T) {
	m := newTestModel()
	delegate := folderDelegate{
		styles:   m.FoldersStyle,
		depths:   map[Folder]int{Folder("work"): 0},
		expanded: map[Folder]bool{},
		children: map[Folder][]Folder{},
		snippets: map[Folder][]Snippet{},
	}
	model := list.New([]list.Item{
		Snippet{
			Name:     "this-is-a-very-long-snippet-name-that-should-be-truncated",
			Folder:   "work",
			File:     "this-is-a-very-long-snippet-name-that-should-be-truncated.md",
			Language: "md",
		},
	}, delegate, 18, 1)

	var out bytes.Buffer
	delegate.Render(&out, model, 0, model.Items()[0])
	rendered := out.String()
	if !strings.Contains(rendered, "...") {
		t.Fatalf("expected ellipsis in rendered tree label, got %q", rendered)
	}
	if strings.Contains(rendered, "\n") {
		t.Fatalf("tree label should stay on one line, got %q", rendered)
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
	if _, ok := msg.(updateFoldersMsg); !ok {
		t.Fatalf("unexpected createNewSnippetFile message type %T", msg)
	}

	items := m.Lists[nestedFolder].Items()
	if len(items) != 1 {
		t.Fatalf("expected one snippet in nested folder, got %d", len(items))
	}
	snippet := items[0].(Snippet)
	if snippet.Name != "01-new-snippet" {
		t.Fatalf("new snippet name mismatch: got %q want %q", snippet.Name, "01-new-snippet")
	}
	path, err := snippetStoragePath(tmp, snippet)
	if err != nil {
		t.Fatalf("could not resolve snippet path: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("nested snippet file was not created: %v", err)
	}
}

func TestCreateNewSnippetFileUsesNextIndex(t *testing.T) {
	tmp := t.TempDir()
	m := newBoundIndexTestModel()
	m.config.Home = tmp
	m.pane = snippetPane
	if err := os.MkdirAll(filepath.Join(tmp, "work"), 0o755); err != nil {
		t.Fatalf("could not create folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "work", "01-index.md"), []byte{}, 0o644); err != nil {
		t.Fatalf("could not write index snippet: %v", err)
	}

	cmd := m.createNewSnippetFile()
	if cmd == nil {
		t.Fatalf("createNewSnippetFile returned nil")
	}
	msg := cmd()
	update, ok := msg.(updateFoldersMsg)
	if !ok {
		t.Fatalf("unexpected createNewSnippetFile message type %T", msg)
	}
	if update.selectedFolderIndex != 0 {
		t.Fatalf("selected folder index mismatch: got %d want 0", update.selectedFolderIndex)
	}

	items := m.Lists[Folder("work")].Items()
	if len(items) != 2 {
		t.Fatalf("expected two snippets in folder, got %d", len(items))
	}
	if got := items[1].(Snippet).Name; got != "02-new-snippet" {
		t.Fatalf("new snippet name mismatch: got %q want %q", got, "02-new-snippet")
	}
}

func TestCreateNewSnippetFileUsesNextIndexAcrossFoldersAndSnippets(t *testing.T) {
	m := newBoundIndexTestModel()
	m.pane = folderPane
	m.Lists[Folder("work/02-tools")] = newList([]list.Item{
		Snippet{
			Name:     "01-index",
			Folder:   "work/02-tools",
			File:     "01-index.md",
			Language: "md",
		},
	}, 20, m.ListStyle)

	msg := m.updateFoldersView(Folder("work"), false).(updateFoldersMsg)
	m.Folders.SetItems(msg.items)
	m.Folders.Select(msg.selectedFolderIndex)

	cmd := m.createNewSnippetFile()
	if cmd == nil {
		t.Fatalf("createNewSnippetFile returned nil")
	}
	if _, ok := cmd().(updateFoldersMsg); !ok {
		t.Fatalf("unexpected createNewSnippetFile message type %T", cmd())
	}

	items := m.Lists[Folder("work")].Items()
	if len(items) != 2 {
		t.Fatalf("expected two snippets in folder, got %d", len(items))
	}
	if got := items[1].(Snippet).Name; got != "03-new-snippet" {
		t.Fatalf("new snippet name mismatch: got %q want %q", got, "03-new-snippet")
	}
}

func TestFolderPaneUppercaseNCreatesIndexedChildFolder(t *testing.T) {
	tmp := t.TempDir()
	m := newBoundIndexTestModel()
	m.config.Home = tmp
	m.pane = folderPane
	m.folderExpanded[Folder("work")] = true

	msg := m.updateFoldersView(Folder("work"), false).(updateFoldersMsg)
	m.Folders.SetItems(msg.items)
	m.Folders.Select(msg.selectedFolderIndex)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	if cmd == nil {
		t.Fatalf("expected folder creation command")
	}

	updated, _ = updated.(*Model).Update(cmd())

	got := updated.(*Model)
	item, ok := got.selectedFolderItem().(Snippet)
	if !ok {
		t.Fatalf("expected new folder index snippet selection, got %T", got.selectedFolderItem())
	}
	if item.Folder != "work/02-new-folder" {
		t.Fatalf("new folder mismatch: got %q want %q", item.Folder, "work/02-new-folder")
	}
	path := filepath.Join(tmp, "work", "02-new-folder", "01-index.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("new folder index snippet was not created: %v", err)
	}
	if string(content) != "# New Folder\n" {
		t.Fatalf("new folder index content mismatch: got %q want %q", string(content), "# New Folder\n")
	}
}

func TestFolderPaneUppercaseOCreatesIndexedRootFolder(t *testing.T) {
	tmp := t.TempDir()
	m := newBoundIndexTestModel()
	m.config.Home = tmp
	m.pane = folderPane
	m.folderExpanded[Folder("work")] = true

	msg := m.updateFoldersView(Folder("work/tools"), false).(updateFoldersMsg)
	m.Folders.SetItems(msg.items)
	m.Folders.Select(msg.selectedFolderIndex)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'O'}})
	if cmd == nil {
		t.Fatalf("expected root folder creation command")
	}
	updated, _ = updated.(*Model).Update(cmd())

	got := updated.(*Model)
	item, ok := got.selectedFolderItem().(Snippet)
	if !ok {
		t.Fatalf("expected root folder index snippet selection, got %T", got.selectedFolderItem())
	}
	if item.Folder != "01-new-folder" {
		t.Fatalf("new root folder mismatch: got %q want %q", item.Folder, "01-new-folder")
	}
	if _, err := os.Stat(filepath.Join(tmp, "01-new-folder", "01-index.md")); err != nil {
		t.Fatalf("root folder index snippet was not created: %v", err)
	}
}

func TestFolderPaneUppercaseXDeletesSelectedFolderSubtree(t *testing.T) {
	tmp := t.TempDir()
	m := newBoundIndexTestModel()
	m.config.Home = tmp
	m.pane = folderPane
	m.folderExpanded[Folder("work")] = true

	for _, path := range []string{
		filepath.Join(tmp, "work", "01-index.md"),
		filepath.Join(tmp, "work", "tools", "01-index.md"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("could not create folder: %v", err)
		}
		if err := os.WriteFile(path, []byte("# index\n"), 0o644); err != nil {
			t.Fatalf("could not write snippet: %v", err)
		}
	}

	msg := m.updateFoldersView(Folder("work/tools"), false).(updateFoldersMsg)
	m.Folders.SetItems(msg.items)
	m.Folders.Select(msg.selectedFolderIndex)
	if m.selectedFolder() != Folder("work/tools") {
		t.Fatalf("precondition selected folder mismatch: got %q want %q", m.selectedFolder(), Folder("work/tools"))
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	if cmd == nil {
		t.Fatalf("expected delete confirmation state change")
	}
	updated, _ = updated.(*Model).Update(cmd())
	got := updated.(*Model)
	if got.pane != folderPane {
		t.Fatalf("pane mismatch: got %v want %v", got.pane, folderPane)
	}
	if got.state != deletingState {
		t.Fatalf("state mismatch: got %v want %v", got.state, deletingState)
	}
	if !strings.Contains(got.View(), "(y/N)") {
		t.Fatalf("delete confirmation should be visible in the view")
	}

	deleteCmd := m.deleteSelectedFolder()
	if deleteCmd == nil {
		t.Fatalf("deleteSelectedFolder returned nil")
	}
	deleteMsg := deleteCmd()
	update, ok := deleteMsg.(updateFoldersMsg)
	if !ok {
		t.Fatalf("unexpected deleteSelectedFolder message type %T", deleteMsg)
	}
	if update.selectedFolderIndex != 0 {
		t.Fatalf("selected folder index mismatch: got %d want 0", update.selectedFolderIndex)
	}
	if _, exists := m.Lists[Folder("work/tools")]; exists {
		t.Fatalf("deleted folder still exists in lists")
	}
	if _, err := os.Stat(filepath.Join(tmp, "work", "tools")); !os.IsNotExist(err) {
		t.Fatalf("deleted folder still exists on disk: %v", err)
	}
}

func TestSnippetDeletionPromptDisplaysConfirmation(t *testing.T) {
	m := newBoundIndexTestModel()
	m.folderExpanded[Folder("work")] = true

	msg := m.updateFoldersView(Snippet{
		Name:     "01-index",
		Folder:   "work",
		File:     "01-index.md",
		Language: "md",
	}, false).(updateFoldersMsg)
	m.Folders.SetItems(msg.items)
	m.Folders.Select(msg.selectedFolderIndex)
	m.updateKeyMap()

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("expected delete confirmation state change")
	}
	updated, _ = updated.(*Model).Update(cmd())
	got := updated.(*Model)
	if got.state != deletingState {
		t.Fatalf("state mismatch: got %v want %v", got.state, deletingState)
	}
	if !strings.Contains(got.View(), "(y/N)") {
		t.Fatalf("snippet delete confirmation should be visible in the view")
	}
}

func TestFolderPaneLeftExpandsTreeBeforeDescending(t *testing.T) {
	m := newNestedFolderTestModel()

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if cmd == nil {
		t.Fatalf("expected folder tree expansion command")
	}

	updated, cmd = updated.(*Model).Update(cmd())
	got := updated.(*Model)
	if got.selectedFolder() != Folder("work") {
		t.Fatalf("selected folder mismatch: got %q want %q", got.selectedFolder(), Folder("work"))
	}
	if !got.folderExpanded[Folder("work")] {
		t.Fatalf("expected parent folder to be expanded")
	}
	if len(got.Folders.Items()) != 2 {
		t.Fatalf("visible folder count mismatch: got %d want 2", len(got.Folders.Items()))
	}
}

func TestFolderPaneLeftOnLeafExpandsToShowSnippetChildren(t *testing.T) {
	m := newNestedFolderTestModel()
	m.folderExpanded[Folder("work")] = true

	msg := m.updateFoldersView(Folder("work/backend"), false).(updateFoldersMsg)
	m.Folders.SetItems(msg.items)
	m.Folders.Select(msg.selectedFolderIndex)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	got := updated.(*Model)
	if got.pane != folderPane {
		t.Fatalf("pane mismatch: got %v want %v", got.pane, folderPane)
	}
	if !got.folderExpanded[Folder("work/backend")] {
		t.Fatalf("expected leaf folder with snippets to expand")
	}
	if cmd == nil {
		t.Fatalf("expected folder expansion command")
	}
}

func TestFolderPaneLExpandsTreeBeforeDescending(t *testing.T) {
	m := newNestedFolderTestModel()

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if cmd == nil {
		t.Fatalf("expected folder tree expansion command")
	}

	updated, _ = updated.(*Model).Update(cmd())
	got := updated.(*Model)
	if got.selectedFolder() != Folder("work") {
		t.Fatalf("selected folder mismatch: got %q want %q", got.selectedFolder(), Folder("work"))
	}
	if !got.folderExpanded[Folder("work")] {
		t.Fatalf("expected parent folder to be expanded")
	}
}

func TestFolderPaneLeftOnExpandedFolderWithSnippetsSelectsFirstSnippet(t *testing.T) {
	m := newBoundIndexTestModel()
	m.pane = folderPane
	m.folderExpanded[Folder("work")] = true

	msg := m.updateFoldersView(Folder("work"), false).(updateFoldersMsg)
	m.Folders.SetItems(msg.items)
	m.Folders.Select(msg.selectedFolderIndex)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if cmd == nil {
		t.Fatalf("expected tree selection command")
	}
	updated, _ = updated.(*Model).Update(cmd())
	got := updated.(*Model)
	snippet, ok := got.selectedFolderItem().(Snippet)
	if !ok {
		t.Fatalf("expected snippet selection, got %T", got.selectedFolderItem())
	}
	if snippet.Folder != "work" || snippet.Name != "01-index" {
		t.Fatalf("selected snippet mismatch: got %q/%q want %q/%q", snippet.Folder, snippet.Name, "work", "01-index")
	}
}

func TestFolderPaneLowercaseNCreatesSnippetInSelectedRootFolder(t *testing.T) {
	tmp := t.TempDir()
	m := newBoundIndexTestModel()
	m.config.Home = tmp
	m.pane = folderPane
	m.folderExpanded[Folder("work")] = true
	if err := os.MkdirAll(filepath.Join(tmp, "work"), 0o755); err != nil {
		t.Fatalf("could not create folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "work", "01-index.md"), []byte{}, 0o644); err != nil {
		t.Fatalf("could not write root index snippet: %v", err)
	}

	msg := m.updateFoldersView(Folder("work"), false).(updateFoldersMsg)
	m.Folders.SetItems(msg.items)
	m.Folders.Select(msg.selectedFolderIndex)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if cmd == nil {
		t.Fatalf("expected snippet creation command")
	}
	if _, ok := cmd().(updateFoldersMsg); !ok {
		t.Fatalf("unexpected snippet creation message type %T", cmd())
	}

	got := updated.(*Model)
	items := got.Lists[Folder("work")].Items()
	if len(items) != 2 {
		t.Fatalf("expected two snippets in root folder, got %d", len(items))
	}
	if gotName := items[1].(Snippet).Name; gotName != "02-new-snippet" {
		t.Fatalf("new root snippet mismatch: got %q want %q", gotName, "02-new-snippet")
	}
	if gotFolder := items[1].(Snippet).Folder; gotFolder != "work" {
		t.Fatalf("new root snippet folder mismatch: got %q want %q", gotFolder, "work")
	}

	msg = got.updateFoldersView(Folder("work"), false).(updateFoldersMsg)
	if len(msg.items) != 4 {
		t.Fatalf("visible tree item count mismatch: got %d want 4", len(msg.items))
	}
	if snippet, ok := msg.items[2].(Snippet); !ok || snippet.Name != "02-new-snippet" {
		t.Fatalf("expected new snippet in tree at index 2, got %T %v", msg.items[2], msg.items[2])
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

func TestFolderTreeShowsSnippetsBeforeChildFolder(t *testing.T) {
	m := newBoundIndexTestModel()
	m.folderExpanded[Folder("work")] = true

	msg := m.updateFoldersView(Folder("work"), false).(updateFoldersMsg)
	items := msg.items
	if len(items) != 3 {
		t.Fatalf("visible item count mismatch: got %d want 3", len(items))
	}
	if snippet, ok := items[1].(Snippet); !ok || snippet.Name != "01-index" {
		t.Fatalf("expected snippet item at index 1, got %T %v", items[1], items[1])
	}
	if child, ok := items[2].(Folder); !ok || child != Folder("work/tools") {
		t.Fatalf("expected child folder at index 2, got %T %v", items[2], items[2])
	}
}

func TestFolderTreeUsesSharedMixedIndexOrder(t *testing.T) {
	m := newBoundIndexTestModel()
	delete(m.Lists, Folder("work/tools"))
	m.Lists[Folder("work")] = newList([]list.Item{
		Snippet{
			Name:     "01-index",
			Folder:   "work",
			File:     "01-index.md",
			Language: "md",
		},
		Snippet{
			Name:     "03-new-snippet",
			Folder:   "work",
			File:     "03-new-snippet.md",
			Language: "md",
		},
	}, 20, m.ListStyle)
	m.Lists[Folder("work/02-clangd")] = newList([]list.Item{
		Snippet{
			Name:     "01-index",
			Folder:   "work/02-clangd",
			File:     "01-index.md",
			Language: "md",
		},
	}, 20, m.ListStyle)
	m.Lists[Folder("work/04-new-folder")] = newList([]list.Item{
		Snippet{
			Name:     "01-index",
			Folder:   "work/04-new-folder",
			File:     "01-index.md",
			Language: "md",
		},
	}, 20, m.ListStyle)
	m.folderExpanded[Folder("work")] = true

	msg := m.updateFoldersView(Folder("work"), false).(updateFoldersMsg)
	items := msg.items
	if len(items) != 5 {
		t.Fatalf("visible item count mismatch: got %d want 5", len(items))
	}
	if snippet, ok := items[1].(Snippet); !ok || snippet.Name != "01-index" {
		t.Fatalf("expected 01-index at index 1, got %T %v", items[1], items[1])
	}
	if child, ok := items[2].(Folder); !ok || child != Folder("work/02-clangd") {
		t.Fatalf("expected 02-clangd at index 2, got %T %v", items[2], items[2])
	}
	if snippet, ok := items[3].(Snippet); !ok || snippet.Name != "03-new-snippet" {
		t.Fatalf("expected 03-new-snippet at index 3, got %T %v", items[3], items[3])
	}
	if child, ok := items[4].(Folder); !ok || child != Folder("work/04-new-folder") {
		t.Fatalf("expected 04-new-folder at index 4, got %T %v", items[4], items[4])
	}
}

func TestNestedFolderWithOnlyIndexShowsExpandableChild(t *testing.T) {
	m := newBoundIndexTestModel()
	m.folderExpanded[Folder("work")] = true

	msg := m.updateFoldersView(Folder("work/tools"), false).(updateFoldersMsg)
	m.Folders.SetItems(msg.items)
	m.Folders.Select(msg.selectedFolderIndex)

	item := m.Folders.SelectedItem()
	delegate := folderDelegate{
		styles:   m.FoldersStyle,
		depths:   m.folderTree.depths,
		expanded: m.folderExpanded,
		children: m.folderTree.children,
		snippets: m.folderTree.snippets,
	}
	label := delegate.itemLabel(item)
	if !strings.Contains(label, "▸ ") {
		t.Fatalf("expected expandable icon for nested folder with bound index, got %q", label)
	}
}

func TestFolderPaneLeftExpandsFolderBeforeSelectingChildren(t *testing.T) {
	m := newBoundIndexTestModel()

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if cmd == nil {
		t.Fatalf("expected folder tree expansion command")
	}

	updated, _ = updated.(*Model).Update(cmd())
	got := updated.(*Model)
	if _, ok := got.selectedFolderItem().(Folder); !ok {
		t.Fatalf("expected folder selection after expansion, got %T", got.selectedFolderItem())
	}
	if got.selectedFolder() != Folder("work") {
		t.Fatalf("selected folder mismatch: got %q want %q", got.selectedFolder(), Folder("work"))
	}
}

func TestSnippetTreeItemEntersContentPane(t *testing.T) {
	m := newBoundIndexTestModel()
	m.folderExpanded[Folder("work")] = true

	msg := m.updateFoldersView(Snippet{
		Name:     "01-index",
		Folder:   "work",
		File:     "01-index.md",
		Language: "md",
	}, false).(updateFoldersMsg)
	m.Folders.SetItems(msg.items)
	m.Folders.Select(msg.selectedFolderIndex)
	m.syncSelectedTreeSnippet()

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	got := updated.(*Model)
	if got.pane != contentPane {
		t.Fatalf("pane mismatch: got %v want %v", got.pane, contentPane)
	}
	if got.selectedSnippet().Name != "01-index" {
		t.Fatalf("selected snippet mismatch: got %q want %q", got.selectedSnippet().Name, "01-index")
	}
	if cmd != nil {
		if _, ok := cmd().(contentRenderedMsg); !ok {
			t.Fatalf("unexpected follow-up message type %T", cmd())
		}
	}
}

func TestLeafFolderWithIndexExpandsThenSelectsSnippet(t *testing.T) {
	m := newBoundIndexTestModel()
	m.folderExpanded[Folder("work")] = true

	msg := m.updateFoldersView(Folder("work/tools"), false).(updateFoldersMsg)
	m.Folders.SetItems(msg.items)
	m.Folders.Select(msg.selectedFolderIndex)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if cmd == nil {
		t.Fatalf("expected folder expansion command")
	}
	updated, _ = updated.(*Model).Update(cmd())
	got := updated.(*Model)
	if got.pane != folderPane {
		t.Fatalf("pane mismatch after expansion: got %v want %v", got.pane, folderPane)
	}

	updated, cmd = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if cmd == nil {
		t.Fatalf("expected snippet selection command")
	}
	updated, _ = updated.(*Model).Update(cmd())
	got = updated.(*Model)
	snippet, ok := got.selectedFolderItem().(Snippet)
	if !ok {
		t.Fatalf("expected snippet selection, got %T", got.selectedFolderItem())
	}
	if snippet.Folder != "work/tools" || snippet.Name != "01-index" {
		t.Fatalf("selected snippet mismatch: got %q/%q want %q/%q", snippet.Folder, snippet.Name, "work/tools", "01-index")
	}
}

func TestSnippetTreeItemHReturnsToParentFolder(t *testing.T) {
	m := newBoundIndexTestModel()
	m.folderExpanded[Folder("work")] = true

	msg := m.updateFoldersView(Snippet{
		Name:     "01-index",
		Folder:   "work",
		File:     "01-index.md",
		Language: "md",
	}, false).(updateFoldersMsg)
	m.Folders.SetItems(msg.items)
	m.Folders.Select(msg.selectedFolderIndex)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if cmd == nil {
		t.Fatalf("expected parent selection command")
	}

	updated, _ = updated.(*Model).Update(cmd())
	got := updated.(*Model)
	if got.selectedFolder() != Folder("work") {
		t.Fatalf("selected folder mismatch: got %q want %q", got.selectedFolder(), Folder("work"))
	}
	if _, ok := got.selectedFolderItem().(Folder); !ok {
		t.Fatalf("expected parent folder to be selected, got %T", got.selectedFolderItem())
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
