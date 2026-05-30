package nap

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/list"
)

const (
	defaultFlashcardsCommand  = "hascard"
	defaultFlashcardDeckStem  = "00-cards"
	flashcardDeckPositiveStem = defaultFlashcardDeckStem + "+"
	flashcardDeckNegativeStem = defaultFlashcardDeckStem + "-"
	defaultFlashcardLanguage  = "txt"
	defaultFlashcardExtension = "." + defaultFlashcardLanguage
)

var (
	errFlashcardsCommandNotFound  = errors.New("flashcards command not found")
	errFlashcardDeckMissing       = errors.New("flashcard deck not found")
	errFlashcardDeckAmbiguous     = errors.New("multiple flashcard decks found")
	errFlashcardDeckResetConflict = errors.New("plain flashcard deck already exists")
)

//go:embed templates/flashcards/00-cards.txt
var defaultFlashcardDeckTemplate string

type flashcardsFinishedMsg struct {
	err         error
	snippetPath string
	preview     bool
}

type flashcardDeckState int

const (
	flashcardDeckPending flashcardDeckState = iota
	flashcardDeckPositive
	flashcardDeckNegative
)

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

func isHiddenFlashcardDeck(snippet Snippet) bool {
	state, ok := flashcardDeckStateForSnippet(snippet)
	return ok && state != flashcardDeckPending
}

func isFlashcardDeckFile(file string) bool {
	name := filepath.Base(file)
	if _, ok := flashcardDeckStateForFile(name); !ok {
		return false
	}

	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".txt":
		return true
	}

	return false
}

func flashcardDeckStateForSnippet(snippet Snippet) (flashcardDeckState, bool) {
	return flashcardDeckStateForFile(snippet.File)
}

func flashcardDeckStateForFile(file string) (flashcardDeckState, bool) {
	name := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	switch name {
	case defaultFlashcardDeckStem:
		return flashcardDeckPending, true
	case flashcardDeckPositiveStem:
		return flashcardDeckPositive, true
	case flashcardDeckNegativeStem:
		return flashcardDeckNegative, true
	default:
		return 0, false
	}
}

func flashcardDeckStatusLabel(state flashcardDeckState) string {
	switch state {
	case flashcardDeckPositive:
		return "answered correctly"
	case flashcardDeckNegative:
		return "answered incorrectly"
	default:
		return "ready to review"
	}
}

type flashcardDeckSelection struct {
	pending  []Snippet
	positive []Snippet
	negative []Snippet
}

func classifyFlashcardDecks(decks []Snippet) flashcardDeckSelection {
	selection := flashcardDeckSelection{}
	for _, deck := range decks {
		state, ok := flashcardDeckStateForSnippet(deck)
		if !ok {
			continue
		}
		switch state {
		case flashcardDeckPositive:
			selection.positive = append(selection.positive, deck)
		case flashcardDeckNegative:
			selection.negative = append(selection.negative, deck)
		default:
			selection.pending = append(selection.pending, deck)
		}
	}
	return selection
}

func (s flashcardDeckSelection) answered() []Snippet {
	answered := make([]Snippet, 0, len(s.positive)+len(s.negative))
	answered = append(answered, s.positive...)
	answered = append(answered, s.negative...)
	return answered
}

func (s flashcardDeckSelection) reviewDeck() (Snippet, error) {
	switch len(s.pending) {
	case 0:
	case 1:
		if len(s.positive) > 0 && len(s.negative) > 0 {
			return Snippet{}, errFlashcardDeckAmbiguous
		}
		return s.pending[0], nil
	default:
		return Snippet{}, errFlashcardDeckAmbiguous
	}

	answered := s.answered()
	switch len(answered) {
	case 0:
		return Snippet{}, errFlashcardDeckMissing
	case 1:
		return answered[0], nil
	default:
		return Snippet{}, errFlashcardDeckAmbiguous
	}
}

func (s flashcardDeckSelection) canReset() bool {
	return len(s.answered()) > 0
}

func (s flashcardDeckSelection) dashboardStatus() string {
	switch {
	case len(s.pending) == 0 && len(s.positive) == 0 && len(s.negative) == 0:
		return "not configured"
	case len(s.pending) > 1 || len(s.positive) > 1 || len(s.negative) > 1:
		return "multiple decks found"
	case len(s.pending) == 1 && len(s.positive) == 0 && len(s.negative) == 0:
		return flashcardDeckStatusLabel(flashcardDeckPending)
	case len(s.pending) == 0 && len(s.positive) == 1 && len(s.negative) == 0:
		return flashcardDeckStatusLabel(flashcardDeckPositive)
	case len(s.pending) == 0 && len(s.positive) == 0 && len(s.negative) == 1:
		return flashcardDeckStatusLabel(flashcardDeckNegative)
	case len(s.pending) == 1 && len(s.positive) == 1 && len(s.negative) == 0:
		return "ready + answered correctly"
	case len(s.pending) == 1 && len(s.positive) == 0 && len(s.negative) == 1:
		return "ready + answered incorrectly"
	case len(s.pending) == 0 && len(s.positive) == 1 && len(s.negative) == 1:
		return "answered correctly + incorrectly"
	case len(s.pending) == 1 && len(s.positive) == 1 && len(s.negative) == 1:
		return "ready + answered cards"
	default:
		return "multiple decks found"
	}
}

func resetFlashcardDeckSnippet(snippet Snippet) Snippet {
	reset := snippet
	reset.Name = defaultFlashcardDeckStem
	reset.File = defaultFlashcardDeckStem + filepath.Ext(snippet.File)
	return reset
}

func resetFlashcardDeckOnDisk(home string, snippet Snippet) (Snippet, error) {
	reset := resetFlashcardDeckSnippet(snippet)
	oldPath, err := snippetStoragePath(home, snippet)
	if err != nil {
		return Snippet{}, err
	}
	newPath, err := snippetStoragePath(home, reset)
	if err != nil {
		return Snippet{}, err
	}
	if oldPath == newPath {
		return reset, nil
	}
	if _, err := os.Stat(newPath); err == nil {
		return Snippet{}, errFlashcardDeckResetConflict
	} else if !errors.Is(err, os.ErrNotExist) {
		return Snippet{}, err
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return Snippet{}, err
	}
	return reset, nil
}

func resetFlashcardDecksOnDisk(home string, decks []Snippet) (Snippet, []Snippet, error) {
	selection := classifyFlashcardDecks(decks)
	answered := selection.answered()
	if len(answered) == 0 {
		return Snippet{}, nil, errFlashcardDeckMissing
	}

	target := Snippet{}
	var existingContent []byte
	if len(selection.pending) > 0 {
		target = selection.pending[0]
		path, err := snippetStoragePath(home, target)
		if err != nil {
			return Snippet{}, nil, err
		}
		existingContent, err = os.ReadFile(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return Snippet{}, nil, err
		}
	} else {
		target = resetFlashcardDeckSnippet(answered[0])
	}

	parts := make([][]byte, 0, len(answered)+1)
	if len(bytes.TrimSpace(existingContent)) > 0 {
		parts = append(parts, bytes.TrimRight(existingContent, "\n"))
	}

	for _, deck := range answered {
		path, err := snippetStoragePath(home, deck)
		if err != nil {
			return Snippet{}, nil, err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return Snippet{}, nil, err
		}
		if len(bytes.TrimSpace(content)) == 0 {
			continue
		}
		parts = append(parts, bytes.TrimRight(content, "\n"))
	}

	targetPath, err := snippetStoragePath(home, target)
	if err != nil {
		return Snippet{}, nil, err
	}
	merged := bytes.Join(parts, []byte("\n\n"))
	if err := os.WriteFile(targetPath, merged, 0o644); err != nil {
		return Snippet{}, nil, err
	}

	for _, deck := range answered {
		path, err := snippetStoragePath(home, deck)
		if err != nil {
			return Snippet{}, nil, err
		}
		if path == targetPath {
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return Snippet{}, nil, err
		}
	}

	return target, answered, nil
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
