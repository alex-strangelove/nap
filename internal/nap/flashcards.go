package nap

import (
	"errors"
	"path/filepath"

	"github.com/charmbracelet/bubbles/list"
)

const (
	defaultFlashcardDeckStem  = nativeFlashcardDeckStem
	defaultFlashcardLanguage  = "md"
	defaultFlashcardExtension = "." + defaultFlashcardLanguage
)

var (
	errFlashcardDeckMissing   = errors.New("flashcard deck not found")
	errFlashcardDeckAmbiguous = errors.New("multiple flashcard decks found")
)

type flashcardDeckState int

const (
	flashcardDeckPending flashcardDeckState = iota
	flashcardDeckRecall
	flashcardDeckPositive
	flashcardDeckNegative
)

func isFlashcardDeck(snippet Snippet) bool {
	return isFlashcardDeckFile(snippet.File)
}

func isHiddenFlashcardDeck(snippet Snippet) bool {
	return false
}

func isFlashcardDeckFile(file string) bool {
	return isNativeFlashcardDeckFile(filepath.Base(file))
}

func flashcardDeckStateForSnippet(snippet Snippet) (flashcardDeckState, bool) {
	return flashcardDeckStateForFile(snippet.File)
}

func flashcardDeckStateForFile(file string) (flashcardDeckState, bool) {
	if isNativeFlashcardDeckFile(file) {
		return flashcardDeckPending, true
	}
	return 0, false
}

func flashcardDeckStatusLabel(state flashcardDeckState) string {
	switch state {
	case flashcardDeckRecall:
		return "answered with effort"
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
	return nil
}

func (s flashcardDeckSelection) reviewDeck() (Snippet, error) {
	switch len(s.pending) {
	case 0:
		return Snippet{}, errFlashcardDeckMissing
	case 1:
		return s.pending[0], nil
	default:
		return Snippet{}, errFlashcardDeckAmbiguous
	}
}

func (s flashcardDeckSelection) canReset() bool {
	return false
}

func (s flashcardDeckSelection) dashboardStatus() string {
	switch {
	case len(s.pending) == 0:
		return "not configured"
	case len(s.pending) > 1:
		return "multiple decks found"
	case len(s.pending) == 1:
		return flashcardDeckStatusLabel(flashcardDeckPending)
	default:
		return "multiple decks found"
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
