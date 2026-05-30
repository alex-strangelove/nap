package nap

import (
	"errors"
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
		"00-cards+.md":  true,
		"00-cards-.txt": true,
		"00-review.md":  false,
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
		Snippet{Name: "00-cards+", Folder: defaultSnippetFolder, File: "00-cards+.txt", Language: "txt", Date: time.Now()},
	}, 20, m.ListStyle)

	if _, err := m.currentFlashcardDeck(); err == nil {
		t.Fatal("expected multiple decks to be rejected")
	}
}

func TestReviewDeckPrefersPendingDeckWhenAnsweredDeckAlsoExists(t *testing.T) {
	deck, err := classifyFlashcardDecks([]Snippet{
		{Name: "00-cards", Folder: defaultSnippetFolder, File: "00-cards.txt", Language: "txt", Date: time.Now()},
		{Name: "00-cards+", Folder: defaultSnippetFolder, File: "00-cards+.txt", Language: "txt", Date: time.Now()},
	}).reviewDeck()
	if err != nil {
		t.Fatalf("expected pending deck to stay reviewable, got %v", err)
	}
	if deck.File != "00-cards.txt" {
		t.Fatalf("expected pending deck to be reviewed first, got %q", deck.File)
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

func TestResetFlashcardsRenamesAnsweredDeckToPlainDeck(t *testing.T) {
	tmp := tmpHome(t)
	m := newTestModel()
	m.config.Home = tmp
	m.config.FlashcardsEnabled = true
	folder := Folder(defaultSnippetFolder)
	deck := Snippet{
		Name:     "00-cards+",
		Folder:   defaultSnippetFolder,
		File:     "00-cards+.txt",
		Language: "txt",
		Date:     time.Now(),
	}
	if err := os.MkdirAll(filepath.Join(tmp, defaultSnippetFolder), 0o755); err != nil {
		t.Fatalf("could not create deck folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, defaultSnippetFolder, deck.File), []byte("# cards"), 0o644); err != nil {
		t.Fatalf("could not write answered deck: %v", err)
	}
	m.Lists[folder] = newList([]list.Item{deck}, 20, m.ListStyle)
	m.Folders.SetItems([]list.Item{folder})
	m.Folders.Select(0)
	m.updateKeyMap()

	msg := m.resetFlashcards()()
	if _, ok := msg.(updateFoldersMsg); !ok {
		t.Fatalf("reset flashcards message mismatch: got %#v", msg)
	}

	decks := m.flashcardDecks(folder)
	if len(decks) != 1 {
		t.Fatalf("expected one flashcard deck after reset, got %d", len(decks))
	}
	if decks[0].File != "00-cards.txt" {
		t.Fatalf("expected plain flashcard deck after reset, got %q", decks[0].File)
	}
	if _, err := os.Stat(filepath.Join(tmp, defaultSnippetFolder, "00-cards.txt")); err != nil {
		t.Fatalf("expected plain flashcard deck on disk, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, defaultSnippetFolder, "00-cards+.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected answered flashcard deck to be renamed away, got err=%v", err)
	}
}

func TestResetFlashcardDecksOnDiskMergesAnsweredCardsIntoPendingDeck(t *testing.T) {
	tmp := tmpHome(t)
	folder := defaultSnippetFolder
	if err := os.MkdirAll(filepath.Join(tmp, folder), 0o755); err != nil {
		t.Fatalf("could not create deck folder: %v", err)
	}

	pending := Snippet{Name: "00-cards", Folder: folder, File: "00-cards.txt", Language: "txt", Date: time.Now()}
	positive := Snippet{Name: "00-cards+", Folder: folder, File: "00-cards+.txt", Language: "txt", Date: time.Now()}
	negative := Snippet{Name: "00-cards-", Folder: folder, File: "00-cards-.txt", Language: "txt", Date: time.Now()}
	if err := os.WriteFile(filepath.Join(tmp, folder, pending.File), []byte("pending"), 0o644); err != nil {
		t.Fatalf("could not write pending deck: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, folder, positive.File), []byte("correct"), 0o644); err != nil {
		t.Fatalf("could not write positive deck: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, folder, negative.File), []byte("incorrect"), 0o644); err != nil {
		t.Fatalf("could not write negative deck: %v", err)
	}

	reset, answered, err := resetFlashcardDecksOnDisk(tmp, []Snippet{pending, positive, negative})
	if err != nil {
		t.Fatalf("expected reset merge to succeed, got %v", err)
	}
	if reset.File != pending.File {
		t.Fatalf("expected reset to keep plain deck file, got %q", reset.File)
	}
	if len(answered) != 2 {
		t.Fatalf("expected both answered decks to be reset, got %d", len(answered))
	}

	content, err := os.ReadFile(filepath.Join(tmp, folder, pending.File))
	if err != nil {
		t.Fatalf("could not read merged deck: %v", err)
	}
	if string(content) != "pending\n\ncorrect\n\nincorrect" {
		t.Fatalf("merged deck content mismatch: got %q", string(content))
	}
	for _, file := range []string{positive.File, negative.File} {
		if _, err := os.Stat(filepath.Join(tmp, folder, file)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected answered deck %q to be removed, got err=%v", file, err)
		}
	}
}

func TestUpdateContentShowsDashboardWhenFolderContainsHiddenFlashcards(t *testing.T) {
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

	cmd := m.updateContent()
	if cmd == nil {
		return
	}
	if got := fmt.Sprintf("%T", cmd()); got == "tea.execMsg" {
		t.Fatalf("expected folder dashboard update, got flashcard exec command")
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

func TestOpenFlashcardsOnPaneLeftIgnoresHiddenDecksWhenFolderIsSelected(t *testing.T) {
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

	if cmd := m.openFlashcardsOnPaneLeft(); cmd != nil {
		t.Fatalf("expected hidden flashcards to stay off the tree/dashboard flow, got %T", cmd())
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
