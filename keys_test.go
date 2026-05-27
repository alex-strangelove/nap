package main

import (
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

func newTestModel() *Model {
	config := newConfig()
	styles := DefaultStyles(config)
	snippet := defaultSnippet

	folderList := list.New([]list.Item{Folder(defaultSnippetFolder)}, folderDelegate{styles.Folders.Blurred}, 0, 0)
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
