package nap

import (
	"os"
	"path/filepath"
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
	m.enterSearchMode()
	m.refreshSearchResults()
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
