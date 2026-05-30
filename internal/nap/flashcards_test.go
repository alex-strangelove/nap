package nap

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
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
