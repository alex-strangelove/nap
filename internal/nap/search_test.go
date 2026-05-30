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

func TestFindSnippetSearchesFileContents(t *testing.T) {
	tmpHome(t)
	cfg := readConfig()

	snippets := []Snippet{
		{
			Name:     "roadmap",
			Folder:   "plans",
			File:     "roadmap.md",
			Language: "md",
		},
		{
			Name:     "notes",
			Folder:   "misc",
			File:     "notes.go",
			Language: "go",
		},
	}

	writeTestSnippetFile(t, cfg.Home, snippets[0], "# Roadmap\nrollback checklist\n")
	writeTestSnippetFile(t, cfg.Home, snippets[1], "package main\n")

	got := findSnippet(cfg, "rollback checklist", snippets)
	if got.Path() != snippets[0].Path() {
		t.Fatalf("content search mismatch: got %q want %q", got.Path(), snippets[0].Path())
	}
}

func TestSearchModeSearchesContentsAndAppliesSelection(t *testing.T) {
	m, snippets := newSearchTestModel(t)
	m.enterSearchMode(contentSearchMode, false)
	m.searchInput.SetValue("rollback checklist")
	m.refreshSearchResults()

	selected, ok := m.selectedSearchSnippet()
	if !ok || selected.Path() != snippets[0].Path() {
		t.Fatalf("search selection mismatch: got %#v want %q", selected, snippets[0].Path())
	}

	cmd := m.exitSearchMode(true)
	if cmd == nil {
		t.Fatal("expected apply-selection command")
	}
	msg := cmd()
	if msg == nil {
		t.Fatal("expected update message")
	}
	if _, followup := m.Update(msg); followup != nil {
		if next := followup(); next != nil {
			m.Update(next)
		}
	}

	if m.state != navigatingState {
		t.Fatalf("state mismatch after leaving search: got %v want %v", m.state, navigatingState)
	}
	selectedSnippet, ok := m.selectedFolderItem().(Snippet)
	if !ok || selectedSnippet.Path() != snippets[0].Path() {
		t.Fatalf("tree selection mismatch after leaving search: got %#v want %q", m.selectedFolderItem(), snippets[0].Path())
	}
}

func TestSearchModeCtrlJKNavigatesResults(t *testing.T) {
	m, _ := newSearchTestModel(t)
	m.enterSearchMode(contentSearchMode, false)

	initial := m.selectedSnippet().Path()
	if initial == "" {
		t.Fatal("expected an initial search selection")
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	if cmd != nil {
		if next := cmd(); next != nil {
			updated, _ = updated.(*Model).Update(next)
		}
	}
	got := updated.(*Model)
	afterDown := got.selectedSnippet().Path()
	if afterDown == initial {
		t.Fatalf("ctrl+j should move selection: still at %q", afterDown)
	}

	updated, cmd = got.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	if cmd != nil {
		if next := cmd(); next != nil {
			updated, _ = updated.(*Model).Update(next)
		}
	}
	got = updated.(*Model)
	if selected := got.selectedSnippet().Path(); selected != initial {
		t.Fatalf("ctrl+k selection mismatch: got %q want %q", selected, initial)
	}
}

func TestSearchModeTypingSearchKeysDoesNotSwitchScope(t *testing.T) {
	m, _ := newSearchTestModel(t)
	m.enterSearchMode(previewSearchMode, false)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if cmd != nil {
		if next := cmd(); next != nil {
			updated, _ = updated.(*Model).Update(next)
		}
	}

	got := updated.(*Model)
	if got.searchMode != previewSearchMode {
		t.Fatalf("search mode changed unexpectedly: got %v want %v", got.searchMode, previewSearchMode)
	}
	if got.searchInput.Value() != "s" {
		t.Fatalf("expected lowercase search key to update input, got %q", got.searchInput.Value())
	}

	updated, cmd = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("S")})
	if cmd != nil {
		if next := cmd(); next != nil {
			updated, _ = updated.(*Model).Update(next)
		}
	}

	got = updated.(*Model)
	if got.searchMode != previewSearchMode {
		t.Fatalf("search mode changed unexpectedly after uppercase input: got %v want %v", got.searchMode, previewSearchMode)
	}
	if got.searchInput.Value() != "sS" {
		t.Fatalf("expected uppercase search key to update input, got %q", got.searchInput.Value())
	}
}

func TestMetadataSearchModeOnlyMatchesFoldersAndFiles(t *testing.T) {
	m, snippets := newSearchTestModel(t)
	m.enterSearchMode(metadataSearchMode, false)
	m.searchInput.SetValue("rollback checklist")
	m.refreshSearchResults()

	if _, ok := m.selectedSearchSnippet(); ok {
		t.Fatalf("metadata search should ignore file contents")
	}

	m.searchInput.SetValue("roadmap")
	m.refreshSearchResults()

	selected, ok := m.selectedSearchSnippet()
	if !ok || selected.Path() != snippets[0].Path() {
		t.Fatalf("metadata search mismatch: got %#v want %q", selected, snippets[0].Path())
	}
}

func TestPreviewSearchModeKeepsCurrentSnippet(t *testing.T) {
	m, snippets := newSearchTestModel(t)
	m.enterSearchMode(contentSearchMode, false)
	m.searchInput.SetValue("package")
	m.refreshSearchResults()

	if selected, ok := m.selectedSearchSnippet(); !ok || selected.Path() != snippets[1].Path() {
		t.Fatalf("expected content search to select %q, got %#v", snippets[1].Path(), selected)
	}

	m.enterSearchMode(previewSearchMode, true)

	if selected := m.selectedSnippet(); selected.Path() != snippets[0].Path() {
		t.Fatalf("preview search should stay on current file: got %q want %q", selected.Path(), snippets[0].Path())
	}
	if got := m.leftPaneView(); got != m.Folders.View() {
		t.Fatalf("preview search should keep folder pane visible")
	}
}

func TestContentSearchModeHighlightsPreview(t *testing.T) {
	m, _ := newSearchTestModel(t)
	m.enterSearchMode(contentSearchMode, false)
	m.searchInput.SetValue("rollback checklist")
	m.refreshSearchResults()

	highlighted := m.previewContent("# Roadmap\nrollback checklist\n")
	if !strings.Contains(highlighted, ansiResetBackground) {
		t.Fatalf("expected content search preview highlight, got %q", highlighted)
	}
}

func TestContentSearchModeFocusSwitchesBetweenResultsAndPreview(t *testing.T) {
	m, _ := newSearchTestModel(t)
	m.enterSearchMode(contentSearchMode, false)
	m.searchInput.SetValue("rollback checklist")
	m.refreshSearchResults()

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	if cmd != nil {
		if next := cmd(); next != nil {
			updated, _ = updated.(*Model).Update(next)
		}
	}

	got := updated.(*Model)
	if got.pane != contentPane {
		t.Fatalf("expected ctrl+l to focus preview, got pane %v", got.pane)
	}
	if highlighted := got.previewContent("# Roadmap\nrollback checklist\n"); !strings.Contains(highlighted, ansiResetBackground) {
		t.Fatalf("expected highlight to remain visible in preview focus, got %q", highlighted)
	}

	updated, cmd = got.Update(tea.KeyMsg{Type: tea.KeyCtrlH})
	if cmd != nil {
		if next := cmd(); next != nil {
			updated, _ = updated.(*Model).Update(next)
		}
	}

	got = updated.(*Model)
	if got.pane != folderPane {
		t.Fatalf("expected ctrl+h to focus search results, got pane %v", got.pane)
	}
}

func TestCtrlCQuitsFromSearch(t *testing.T) {
	m, _ := newSearchTestModel(t)
	m.enterSearchMode(contentSearchMode, false)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected quit command")
	}
	if got := updated.(*Model).state; got != quittingState {
		t.Fatalf("expected quitting state, got %v", got)
	}
}

func TestPreviewSearchIgnoresRenderedOnlyMatches(t *testing.T) {
	m, _ := newSearchTestModel(t)
	m.enterSearchMode(previewSearchMode, false)
	m.searchInput.SetValue("package")

	highlighted := m.previewContent("package")
	if highlighted != "package" {
		t.Fatalf("expected rendered-only match to stay unhighlighted, got %q", highlighted)
	}
}

func newSearchTestModel(t *testing.T) (*Model, []Snippet) {
	t.Helper()

	tmpHome(t)
	cfg := readConfig()
	styles := DefaultStyles(cfg)

	snippets := []Snippet{
		{
			Name:     "roadmap",
			Folder:   "plans",
			File:     "roadmap.md",
			Language: "md",
		},
		{
			Name:     "notes",
			Folder:   "misc",
			File:     "notes.go",
			Language: "go",
		},
	}

	writeTestSnippetFile(t, cfg.Home, snippets[0], "# Roadmap\nrollback checklist\n")
	writeTestSnippetFile(t, cfg.Home, snippets[1], "package main\n")

	lists := map[Folder]*list.Model{
		Folder("plans"): newList([]list.Item{snippets[0]}, 20, styles.Snippets.Focused),
		Folder("misc"):  newList([]list.Item{snippets[1]}, 20, styles.Snippets.Focused),
	}
	tree := buildFolderTree(lists)
	folderExpanded := map[Folder]bool{
		Folder("plans"): true,
		Folder("misc"):  true,
	}
	folderItems := tree.visibleItems(folderExpanded)
	folderList := list.New(folderItems, folderDelegate{
		styles:   styles.Folders.Focused,
		depths:   tree.depths,
		expanded: folderExpanded,
		children: tree.children,
		snippets: tree.snippets,
	}, 0, 0)
	folderList.Select(visibleFolderIndex(folderItems, Folder("plans"), tree.parents))

	m := &Model{
		config:         cfg,
		keys:           DefaultKeyMap,
		help:           help.New(),
		Lists:          lists,
		Folders:        folderList,
		folderTree:     tree,
		folderExpanded: folderExpanded,
		Code:           viewport.New(80, 0),
		LineNumbers:    viewport.New(5, 0),
		ListStyle:      styles.Snippets.Focused,
		FoldersStyle:   styles.Folders.Focused,
		ContentStyle:   styles.Content.Blurred,
		inputs: []textinput.Model{
			newTextInput(defaultSnippetFolder + " "),
			newTextInput(defaultSnippetName + " "),
			newTextInput(cfg.DefaultLanguage),
		},
		contentCache: map[contentCacheKey]contentCacheEntry{},
		pane:         folderPane,
	}

	m.Init()
	return m, snippets
}

func writeTestSnippetFile(t *testing.T, home string, snippet Snippet, content string) {
	t.Helper()

	path, err := snippetStoragePath(home, snippet)
	if err != nil {
		t.Fatalf("could not derive snippet path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
		t.Fatalf("could not create snippet directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("could not write snippet file: %v", err)
	}
}
