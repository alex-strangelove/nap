package nap

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func TestIsFlashcardDeckFile(t *testing.T) {
	tests := map[string]bool{
		"00-nap-cards.md":  true,
		"00-review.md":     false,
		"000-review.md":    false,
		"00-cards.go":      false,
		"01-cards.txt":     false,
		"00-nap-cards.txt": false,
		"cards.txt":        false,
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
	if deck.File != nativeFlashcardDeckStem+".md" {
		t.Fatalf("flashcard deck file mismatch: got %q", deck.File)
	}

	content, err := os.ReadFile(filepath.Join(tmp, defaultSnippetFolder, deck.File))
	if err != nil {
		t.Fatalf("could not read flashcard deck: %v", err)
	}
	if string(content) != defaultNativeFlashcardDeckContent() {
		t.Fatalf("flashcard deck content mismatch: got %q", string(content))
	}
}

func TestCurrentFlashcardDeckRejectsMultipleDecks(t *testing.T) {
	m := newTestModel()
	m.config.FlashcardsEnabled = true
	folder := Folder(defaultSnippetFolder)
	m.Lists[folder] = newList([]list.Item{
		Snippet{Name: nativeFlashcardDeckStem, Folder: defaultSnippetFolder, File: nativeFlashcardDeckStem + ".md", Language: "md", Date: time.Now()},
		Snippet{Name: nativeFlashcardDeckStem, Folder: defaultSnippetFolder, File: nativeFlashcardDeckStem + ".md", Language: "md", Date: time.Now()},
	}, 20, m.ListStyle)

	if _, err := m.currentFlashcardDeck(); err == nil {
		t.Fatal("expected multiple decks to be rejected")
	}
}

func TestReviewDeckReturnsNativeDeck(t *testing.T) {
	deck, err := classifyFlashcardDecks([]Snippet{
		{Name: nativeFlashcardDeckStem, Folder: defaultSnippetFolder, File: nativeFlashcardDeckStem + ".md", Language: "md", Date: time.Now()},
	}).reviewDeck()
	if err != nil {
		t.Fatalf("expected native deck to be reviewable, got %v", err)
	}
	if deck.File != nativeFlashcardDeckStem+".md" {
		t.Fatalf("expected native deck to be selected, got %q", deck.File)
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

func TestResetNativeFlashcardProgressOnDiskRemovesState(t *testing.T) {
	tmp := tmpHome(t)
	folder := defaultSnippetFolder
	deck := Snippet{Name: nativeFlashcardDeckStem, Folder: folder, File: nativeFlashcardDeckStem + ".md", Language: "md", Date: time.Now()}
	if err := os.MkdirAll(filepath.Join(tmp, folder), 0o755); err != nil {
		t.Fatalf("could not create deck folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, folder, deck.File), []byte(defaultNativeFlashcardDeckContent()), 0o644); err != nil {
		t.Fatalf("could not write native deck: %v", err)
	}
	state := nativeFlashcardState{
		Cards: map[string]nativeFlashcardProgress{
			"linux-paging-4level-walk": {Reviews: 1, LastGrade: flashcardGradeGood, DueAt: time.Now().Add(24 * time.Hour)},
		},
	}
	if err := writeNativeFlashcardState(tmp, deck, state); err != nil {
		t.Fatalf("could not write native state: %v", err)
	}

	reset, err := resetNativeFlashcardProgressOnDisk(tmp, []Snippet{deck})
	if err != nil {
		t.Fatalf("expected native reset to succeed, got %v", err)
	}
	if len(reset) != 1 || reset[0].File != deck.File {
		t.Fatalf("expected native reset to report the deck, got %#v", reset)
	}
	path, err := nativeFlashcardStatePath(tmp, deck)
	if err != nil {
		t.Fatalf("could not resolve native state path: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected native state file to be removed, got err=%v", err)
	}
}

func TestUpdateContentShowsDashboardWhenFolderContainsHiddenFlashcards(t *testing.T) {
	tmp := tmpHome(t)
	m := newTestModel()
	m.config.Home = tmp
	m.config.FlashcardsEnabled = true
	m.pane = contentPane
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	deck := Snippet{
		Name:     nativeFlashcardDeckStem,
		Folder:   defaultSnippetFolder,
		File:     nativeFlashcardDeckStem + ".md",
		Language: "md",
		Date:     time.Now(),
	}
	if err := os.MkdirAll(filepath.Join(tmp, defaultSnippetFolder), 0o755); err != nil {
		t.Fatalf("could not create deck folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, defaultSnippetFolder, deck.File), []byte(defaultNativeFlashcardDeckContent()), 0o644); err != nil {
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
	if got := fmt.Sprintf("%T", cmd()); got != "nap.contentRenderedMsg" {
		t.Fatalf("expected folder dashboard update, got %s", got)
	}
}

func TestPreviousPaneOpensFlashcardsForDeckSnippet(t *testing.T) {
	tmp := tmpHome(t)
	m := newTestModel()
	m.config.Home = tmp
	m.config.FlashcardsEnabled = true
	m.pane = contentPane
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	deck := Snippet{
		Name:     nativeFlashcardDeckStem,
		Folder:   defaultSnippetFolder,
		File:     nativeFlashcardDeckStem + ".md",
		Language: "md",
		Date:     time.Now(),
	}
	if err := os.MkdirAll(filepath.Join(tmp, defaultSnippetFolder), 0o755); err != nil {
		t.Fatalf("could not create deck folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, defaultSnippetFolder, deck.File), []byte(defaultNativeFlashcardDeckContent()), 0o644); err != nil {
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
	got := updated.(*Model)
	if got.pane != contentPane {
		t.Fatalf("expected native review to keep content pane focused, got %v", got.pane)
	}
	if got.state != reviewingFlashcardsState {
		t.Fatalf("expected native review state after left pane switch, got %v", got.state)
	}
}

func TestSnippetTreeRightOpensFlashcards(t *testing.T) {
	tmp := tmpHome(t)
	m := newTestModel()
	m.config.Home = tmp
	m.config.FlashcardsEnabled = true
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	deck := Snippet{
		Name:     nativeFlashcardDeckStem,
		Folder:   defaultSnippetFolder,
		File:     nativeFlashcardDeckStem + ".md",
		Language: "md",
		Date:     time.Now(),
	}
	if err := os.MkdirAll(filepath.Join(tmp, defaultSnippetFolder), 0o755); err != nil {
		t.Fatalf("could not create deck folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, defaultSnippetFolder, deck.File), []byte(defaultNativeFlashcardDeckContent()), 0o644); err != nil {
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
	if got := updated.(*Model); got.pane != contentPane {
		t.Fatalf("expected native review to focus content pane, got %v", got.pane)
	}
	if got := updated.(*Model); got.state != reviewingFlashcardsState {
		t.Fatalf("expected native review state, got %v", got.state)
	}
}

func TestReviewFlashcardsStartsNativeSession(t *testing.T) {
	tmp := tmpHome(t)
	m := newTestModel()
	m.config.Home = tmp
	m.config.FlashcardsEnabled = true
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	deck := Snippet{
		Name:     nativeFlashcardDeckStem,
		Folder:   defaultSnippetFolder,
		File:     nativeFlashcardDeckStem + ".md",
		Language: "md",
		Date:     time.Now(),
	}
	if err := os.MkdirAll(filepath.Join(tmp, defaultSnippetFolder), 0o755); err != nil {
		t.Fatalf("could not create deck folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, defaultSnippetFolder, deck.File), []byte(defaultNativeFlashcardDeckContent()), 0o644); err != nil {
		t.Fatalf("could not write deck: %v", err)
	}
	m.Lists[Folder(defaultSnippetFolder)] = newList([]list.Item{deck}, 20, m.ListStyle)
	m.Folders.SetItems([]list.Item{Folder(defaultSnippetFolder)})
	m.Folders.Select(0)

	cmd := m.reviewFlashcards()
	if cmd == nil {
		t.Fatal("expected native review command")
	}
	if m.state != reviewingFlashcardsState {
		t.Fatalf("expected native review state, got %v", m.state)
	}
	if m.flashcardSession == nil {
		t.Fatal("expected native review session")
	}
}

func TestGradeNativeFlashcardWritesState(t *testing.T) {
	tmp := tmpHome(t)
	m := newTestModel()
	m.config.Home = tmp
	m.config.FlashcardsEnabled = true
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	deck := Snippet{
		Name:     nativeFlashcardDeckStem,
		Folder:   defaultSnippetFolder,
		File:     nativeFlashcardDeckStem + ".md",
		Language: "md",
		Date:     time.Now(),
	}
	if err := os.MkdirAll(filepath.Join(tmp, defaultSnippetFolder), 0o755); err != nil {
		t.Fatalf("could not create deck folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, defaultSnippetFolder, deck.File), []byte(defaultNativeFlashcardDeckContent()), 0o644); err != nil {
		t.Fatalf("could not write deck: %v", err)
	}
	m.Lists[Folder(defaultSnippetFolder)] = newList([]list.Item{deck}, 20, m.ListStyle)
	m.Folders.SetItems([]list.Item{Folder(defaultSnippetFolder)})
	m.Folders.Select(0)

	if cmd := m.reviewFlashcards(); cmd == nil {
		t.Fatal("expected native review command")
	}
	m.flashcardSession.Revealed = true

	if cmd := m.gradeNativeFlashcard(flashcardGradeGood); cmd == nil {
		t.Fatal("expected native grading update")
	}
	path, err := nativeFlashcardStatePath(tmp, deck)
	if err != nil {
		t.Fatalf("could not resolve state path: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected native flashcard state file, got %v", err)
	}
}

func TestStopNativeFlashcardReviewRestoresContent(t *testing.T) {
	tmp := tmpHome(t)
	m := newTestModel()
	m.config.Home = tmp
	m.config.FlashcardsEnabled = true
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	deck := Snippet{
		Name:     nativeFlashcardDeckStem,
		Folder:   defaultSnippetFolder,
		File:     nativeFlashcardDeckStem + ".md",
		Language: "md",
		Date:     time.Now(),
	}
	if err := os.MkdirAll(filepath.Join(tmp, defaultSnippetFolder), 0o755); err != nil {
		t.Fatalf("could not create deck folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, defaultSnippetFolder, deck.File), []byte(defaultNativeFlashcardDeckContent()), 0o644); err != nil {
		t.Fatalf("could not write deck: %v", err)
	}
	m.Lists[Folder(defaultSnippetFolder)] = newList([]list.Item{deck}, 20, m.ListStyle)
	m.Lists[Folder(defaultSnippetFolder)].Select(0)
	m.Folders.SetItems([]list.Item{Folder(defaultSnippetFolder)})
	m.Folders.Select(0)

	cmd := m.reviewFlashcards()
	if cmd == nil {
		t.Fatal("expected native review command")
	}
	m = runModelCmd(m, cmd)
	if m.state != reviewingFlashcardsState {
		t.Fatalf("expected reviewing state before stop review, got %v", m.state)
	}
	got := runModelCmd(m, m.stopNativeFlashcardReview())

	if got.state != navigatingState {
		t.Fatalf("expected navigating state after stop review, got %v", got.state)
	}
	if got.flashcardSession != nil {
		t.Fatal("expected flashcard session to be cleared")
	}
	if strings.Contains(got.Code.View(), "stop review") {
		t.Fatalf("expected review screen to be replaced, got %q", got.Code.View())
	}
	if !strings.Contains(got.Code.View(), "review cards for this folder") {
		t.Fatalf("expected folder dashboard to be restored, got %q", got.Code.View())
	}
}

func TestOpenFlashcardsOnPaneLeftIgnoresHiddenDecksWhenFolderIsSelected(t *testing.T) {
	tmp := tmpHome(t)
	m := newTestModel()
	m.config.Home = tmp
	m.config.FlashcardsEnabled = true
	m.pane = contentPane
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	deck := Snippet{
		Name:     ".00-nap-cards",
		Folder:   defaultSnippetFolder,
		File:     ".00-nap-cards.md",
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
