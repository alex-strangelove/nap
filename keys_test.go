package main

import (
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

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if m.pane != folderPane {
		t.Fatalf("pane after pressing l twice is incorrect: got %v want %v", m.pane, folderPane)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if m.pane != contentPane {
		t.Fatalf("pane after pressing h is incorrect: got %v want %v", m.pane, contentPane)
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

	folderList := list.New([]list.Item{Folder(defaultSnippetFolder)}, folderDelegate{styles: styles.Folders.Blurred}, 0, 0)
	folderList.Select(0)
	lists := map[Folder]*list.Model{
		Folder(defaultSnippetFolder): newList([]list.Item{snippet}, 20, styles.Snippets.Focused),
	}

	m := &Model{
		config:       config,
		keys:         DefaultKeyMap,
		help:         help.New(),
		Lists:        lists,
		Folders:      folderList,
		Code:         viewport.New(80, 0),
		LineNumbers:  viewport.New(5, 0),
		ListStyle:    styles.Snippets.Focused,
		FoldersStyle: styles.Folders.Blurred,
		ContentStyle: styles.Content.Blurred,
		inputs: []textinput.Model{
			newTextInput(defaultSnippetFolder + " "),
			newTextInput(defaultSnippetName + " "),
			newTextInput(config.DefaultLanguage),
		},
	}
	m.updateKeyMap()
	return m
}
