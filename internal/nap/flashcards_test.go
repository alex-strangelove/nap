package nap

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func TestGetFlashcardsCommand(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		command string
		args    []string
	}{
		{
			name:    "default",
			config:  newConfig(),
			command: defaultFlashcardsCommand,
		},
		{
			name: "custom command with flags",
			config: Config{
				FlashcardsCommand: "uvx hascard",
			},
			command: "uvx",
			args:    []string{"hascard"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			command, args := getFlashcardsCommand(tc.config)
			if command != tc.command {
				t.Fatalf("flashcards command mismatch: got %q want %q", command, tc.command)
			}
			if fmt.Sprint(args) != fmt.Sprint(tc.args) {
				t.Fatalf("flashcards args mismatch: got %v want %v", args, tc.args)
			}
		})
	}
}

func TestIsFlashcardDeckFile(t *testing.T) {
	tests := map[string]bool{
		"00-cards.txt":  true,
		"00-review.md":  true,
		"000-review.md": false,
		"00-cards.go":   false,
		"01-cards.txt":  false,
		"cards.txt":     false,
	}

	for file, want := range tests {
		if got := isFlashcardDeckFile(file); got != want {
			t.Fatalf("deck detection mismatch for %q: got %t want %t", file, got, want)
		}
	}
}

func TestNextIndexedMixedNameAfterFlashcardDeckStartsAt01(t *testing.T) {
	items := []list.Item{
		Snippet{Name: defaultFlashcardDeckStem, File: defaultFlashcardDeckStem + defaultFlashcardExtension, Language: defaultFlashcardLanguage},
	}

	if got := nextIndexedMixedName(items, nil, defaultIndexedSnippetStem); got != "01-new-snippet" {
		t.Fatalf("next indexed snippet name mismatch: got %q want %q", got, "01-new-snippet")
	}
}

func TestCreateFlashcardDeck(t *testing.T) {
	tmp := tmpHome(t)
	m := newTestModel()
	m.config.Home = tmp
	m.config.FlashcardsEnabled = true
	m.updateKeyMap()

	msg := m.createFlashcardDeck()()
	if _, ok := msg.(updateFoldersMsg); !ok {
		t.Fatalf("create flashcard deck message mismatch: got %#v", msg)
	}

	decks := m.flashcardDecks(Folder(defaultSnippetFolder))
	if len(decks) != 1 {
		t.Fatalf("expected one flashcard deck, got %d", len(decks))
	}

	deck := decks[0]
	if deck.File != defaultFlashcardDeckStem+defaultFlashcardExtension {
		t.Fatalf("flashcard deck file mismatch: got %q", deck.File)
	}

	content, err := os.ReadFile(filepath.Join(tmp, defaultSnippetFolder, deck.File))
	if err != nil {
		t.Fatalf("could not read flashcard deck: %v", err)
	}
	if string(content) != defaultFlashcardDeckContent() {
		t.Fatalf("flashcard deck content mismatch: got %q", string(content))
	}
}

func TestCurrentFlashcardDeckRejectsMultipleDecks(t *testing.T) {
	m := newTestModel()
	m.config.FlashcardsEnabled = true
	folder := Folder(defaultSnippetFolder)
	m.Lists[folder] = newList([]list.Item{
		Snippet{Name: "00-cards", Folder: defaultSnippetFolder, File: "00-cards.txt", Language: "txt", Date: time.Now()},
		Snippet{Name: "00-alt", Folder: defaultSnippetFolder, File: "00-alt.txt", Language: "txt", Date: time.Now()},
	}, 20, m.ListStyle)

	if _, err := m.currentFlashcardDeck(); err == nil {
		t.Fatal("expected multiple decks to be rejected")
	}
}

func TestReviewFlashcardsDoesNotCreateDeckWhenMissing(t *testing.T) {
	tmp := tmpHome(t)
	m := newTestModel()
	m.config.Home = tmp
	m.config.FlashcardsEnabled = true
	m.updateKeyMap()

	if cmd := m.reviewFlashcards(); cmd != nil {
		t.Fatalf("review should not create a deck when missing, got %v", cmd)
	}

	decks := m.flashcardDecks(Folder(defaultSnippetFolder))
	if len(decks) != 0 {
		t.Fatalf("expected review to leave flashcard decks unchanged, got %d", len(decks))
	}
}

func TestUpdateContentDoesNotAutoPreviewFlashcardDecks(t *testing.T) {
	tmp := tmpHome(t)
	m := newTestModel()
	m.config.Home = tmp
	m.config.FlashcardsEnabled = true
	m.config.FlashcardsCommand = "true"
	m.pane = contentPane
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	deck := Snippet{
		Name:     "00-cards",
		Folder:   defaultSnippetFolder,
		File:     "00-cards.md",
		Language: "md",
		Date:     time.Now(),
	}
	if err := os.MkdirAll(filepath.Join(tmp, defaultSnippetFolder), 0o755); err != nil {
		t.Fatalf("could not create deck folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, defaultSnippetFolder, deck.File), []byte("# cards"), 0o644); err != nil {
		t.Fatalf("could not write deck: %v", err)
	}
	m.Lists[Folder(defaultSnippetFolder)] = newList([]list.Item{deck}, 20, m.ListStyle)
	m.Lists[Folder(defaultSnippetFolder)].Select(0)
	m.Folders.SetItems([]list.Item{deck})
	m.Folders.Select(0)

	cmd := m.updateContent()
	if cmd == nil {
		return
	}
	if got := fmt.Sprintf("%T", cmd()); got == "tea.execMsg" {
		t.Fatalf("expected normal preview update, got flashcard exec command")
	}
}

func TestPreviousPaneOpensFlashcardsForDeckSnippet(t *testing.T) {
	tmp := tmpHome(t)
	m := newTestModel()
	m.config.Home = tmp
	m.config.FlashcardsEnabled = true
	m.config.FlashcardsCommand = "true"
	m.pane = contentPane
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	deck := Snippet{
		Name:     "00-cards",
		Folder:   defaultSnippetFolder,
		File:     "00-cards.md",
		Language: "md",
		Date:     time.Now(),
	}
	if err := os.MkdirAll(filepath.Join(tmp, defaultSnippetFolder), 0o755); err != nil {
		t.Fatalf("could not create deck folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, defaultSnippetFolder, deck.File), []byte("# cards"), 0o644); err != nil {
		t.Fatalf("could not write deck: %v", err)
	}
	m.Lists[Folder(defaultSnippetFolder)] = newList([]list.Item{deck}, 20, m.ListStyle)
	m.Lists[Folder(defaultSnippetFolder)].Select(0)
	m.Folders.SetItems([]list.Item{deck})
	m.Folders.Select(0)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if cmd == nil {
		t.Fatal("expected left pane switch to launch flashcards")
	}
	if got := fmt.Sprintf("%T", cmd()); got != "tea.BatchMsg" {
		t.Fatalf("expected batched pane switch and flashcard launch, got %s", got)
	}
	got := updated.(*Model)
	if got.pane != folderPane {
		t.Fatalf("expected left pane switch to focus folder pane, got %v", got.pane)
	}
}

func TestSnippetTreeRightOpensFlashcards(t *testing.T) {
	tmp := tmpHome(t)
	m := newTestModel()
	m.config.Home = tmp
	m.config.FlashcardsEnabled = true
	m.config.FlashcardsCommand = "true"
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	deck := Snippet{
		Name:     "00-cards",
		Folder:   defaultSnippetFolder,
		File:     "00-cards.md",
		Language: "md",
		Date:     time.Now(),
	}
	if err := os.MkdirAll(filepath.Join(tmp, defaultSnippetFolder), 0o755); err != nil {
		t.Fatalf("could not create deck folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, defaultSnippetFolder, deck.File), []byte("# cards"), 0o644); err != nil {
		t.Fatalf("could not write deck: %v", err)
	}
	m.Lists[Folder(defaultSnippetFolder)] = newList([]list.Item{deck}, 20, m.ListStyle)
	m.Lists[Folder(defaultSnippetFolder)].Select(0)
	m.Folders.SetItems([]list.Item{deck})
	m.Folders.Select(0)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if cmd == nil {
		t.Fatal("expected right to launch flashcards")
	}
	if got := fmt.Sprintf("%T", cmd()); got != "tea.execMsg" {
		t.Fatalf("expected flashcard exec command, got %s", got)
	}
	if got := updated.(*Model); got.pane != folderPane {
		t.Fatalf("expected right to keep focus in folder pane, got %v", got.pane)
	}
}

func TestPreviousPaneOpensFlashcardsForPreviewedDeckWhenFolderIsSelected(t *testing.T) {
	tmp := tmpHome(t)
	m := newTestModel()
	m.config.Home = tmp
	m.config.FlashcardsEnabled = true
	m.config.FlashcardsCommand = "true"
	m.pane = contentPane
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	deck := Snippet{
		Name:     "00-cards",
		Folder:   defaultSnippetFolder,
		File:     "00-cards.md",
		Language: "md",
		Date:     time.Now(),
	}
	if err := os.MkdirAll(filepath.Join(tmp, defaultSnippetFolder), 0o755); err != nil {
		t.Fatalf("could not create deck folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, defaultSnippetFolder, deck.File), []byte("# cards"), 0o644); err != nil {
		t.Fatalf("could not write deck: %v", err)
	}
	m.Lists[Folder(defaultSnippetFolder)] = newList([]list.Item{deck}, 20, m.ListStyle)
	m.Lists[Folder(defaultSnippetFolder)].Select(0)
	m.Folders.SetItems([]list.Item{Folder(defaultSnippetFolder)})
	m.Folders.Select(0)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if cmd == nil {
		t.Fatal("expected h to launch flashcards for the previewed deck")
	}
	if got := fmt.Sprintf("%T", cmd()); got != "tea.BatchMsg" {
		t.Fatalf("expected batched pane switch and flashcard launch, got %s", got)
	}
	if got := updated.(*Model); got.pane != folderPane {
		t.Fatalf("expected h to move focus to folder pane, got %v", got.pane)
	}
}

func TestPreviewFlashcardsReturnsToFolderNavigation(t *testing.T) {
	m := newTestModel()
	m.config.FlashcardsEnabled = true
	m.config.FlashcardsCommand = "hascard"
	m.pane = contentPane
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	deck := Snippet{
		Name:     "00-cards",
		Folder:   defaultSnippetFolder,
		File:     "00-cards.md",
		Language: "md",
		Date:     time.Now(),
	}
	m.Lists[Folder(defaultSnippetFolder)] = newList([]list.Item{deck}, 20, m.ListStyle)
	m.Lists[Folder(defaultSnippetFolder)].Select(0)
	m.Folders.SetItems([]list.Item{deck})
	m.Folders.Select(0)

	updated, cmd := m.Update(flashcardsFinishedMsg{snippetPath: deck.Path(), preview: true})
	if cmd == nil {
		t.Fatal("expected preview return to refresh folder selection")
	}
	if got := updated.(*Model); got.pane != folderPane {
		t.Fatalf("expected preview return to focus folder pane, got %v", got.pane)
	}
}

func TestFlashcardPreviewRelaunchesAfterReturningToPreview(t *testing.T) {
	tmp := tmpHome(t)
	m := newTestModel()
	m.config.Home = tmp
	m.config.FlashcardsEnabled = true
	m.config.FlashcardsCommand = "true"
	m.pane = contentPane
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	deck := Snippet{
		Name:     "00-cards",
		Folder:   defaultSnippetFolder,
		File:     "00-cards.md",
		Language: "md",
		Date:     time.Now(),
	}
	if err := os.MkdirAll(filepath.Join(tmp, defaultSnippetFolder), 0o755); err != nil {
		t.Fatalf("could not create deck folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, defaultSnippetFolder, deck.File), []byte("# cards"), 0o644); err != nil {
		t.Fatalf("could not write deck: %v", err)
	}
	m.Lists[Folder(defaultSnippetFolder)] = newList([]list.Item{deck}, 20, m.ListStyle)
	m.Lists[Folder(defaultSnippetFolder)].Select(0)
	m.Folders.SetItems([]list.Item{deck})
	m.Folders.Select(0)

	updated, _ := m.Update(flashcardsFinishedMsg{snippetPath: deck.Path(), preview: true})
	got := updated.(*Model)

	updated, cmd := got.Update(tea.KeyMsg{Type: tea.KeyTab})
	if cmd == nil {
		t.Fatal("expected tab to switch back to preview")
	}
	got = updated.(*Model)
	if got.pane != contentPane {
		t.Fatalf("expected tab to enter preview, got %v", got.pane)
	}

	updated, cmd = got.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if cmd == nil {
		t.Fatal("expected left pane switch to relaunch flashcards")
	}
	if kind := fmt.Sprintf("%T", cmd()); kind != "tea.BatchMsg" {
		t.Fatalf("expected batched pane switch and flashcard relaunch, got %s", kind)
	}
}
