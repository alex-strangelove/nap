package nap

import (
	_ "embed"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/list"
)

const (
	defaultFlashcardsCommand  = "hascard"
	defaultFlashcardDeckStem  = "00-cards"
	defaultFlashcardLanguage  = "txt"
	defaultFlashcardExtension = "." + defaultFlashcardLanguage
)

var (
	errFlashcardsCommandNotFound = errors.New("flashcards command not found")
	errFlashcardDeckMissing      = errors.New("flashcard deck not found")
	errFlashcardDeckAmbiguous    = errors.New("multiple flashcard decks found")
)

//go:embed templates/flashcards/00-cards.txt
var defaultFlashcardDeckTemplate string

type flashcardsFinishedMsg struct {
	err         error
	snippetPath string
	preview     bool
}

func getFlashcardsCommand(config Config) (string, []string) {
	command := strings.Fields(config.FlashcardsCommand)
	if len(command) > 0 {
		return command[0], command[1:]
	}

	return defaultFlashcardsCommand, nil
}

func flashcardsCmd(config Config, deckPath string) (*exec.Cmd, error) {
	command, args := getFlashcardsCommand(config)
	if _, err := exec.LookPath(command); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("%w: %s", errFlashcardsCommandNotFound, command)
		}
		return nil, err
	}

	return exec.Command(command, append(args, "run", deckPath)...), nil
}

func flashcardsError(config Config, err error) string {
	if errors.Is(err, errFlashcardsCommandNotFound) || errors.Is(err, exec.ErrNotFound) {
		command, _ := getFlashcardsCommand(config)
		return fmt.Sprintf("Install %s or set flashcards_command.", filepathBase(command))
	}

	return fmt.Sprintf("Flashcard review failed: %v", err)
}

func isFlashcardDeck(snippet Snippet) bool {
	return isFlashcardDeckFile(snippet.File)
}

func isFlashcardDeckFile(file string) bool {
	name := filepath.Base(file)
	if !strings.HasPrefix(name, "00-") {
		return false
	}

	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".txt":
		return true
	default:
		return false
	}
}

func flashcardDecks(items []list.Item) []Snippet {
	decks := make([]Snippet, 0, 1)
	for _, item := range items {
		snippet, ok := item.(Snippet)
		if ok && isFlashcardDeck(snippet) {
			decks = append(decks, snippet)
		}
	}

	return decks
}

func defaultFlashcardDeckContent() string {
	return defaultFlashcardDeckTemplate
}
