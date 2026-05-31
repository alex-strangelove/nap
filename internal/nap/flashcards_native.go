package nap

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"gopkg.in/yaml.v3"
)

const (
	nativeFlashcardDeckStem   = "00-nap-cards"
	nativeFlashcardHeaderLine = "<!-- nap-flashcards:v1 -->"
)

var (
	errNativeFlashcardDeckInvalid = errors.New("invalid native flashcard deck")
	errNativeFlashcardNoCardsDue  = errors.New("no flashcards due")
)

//go:embed templates/flashcards/00-nap-cards.md
var defaultNativeFlashcardDeckTemplate string

type nativeFlashcardDeck struct {
	Cards []nativeFlashcard
}

type nativeFlashcard struct {
	ID       string
	Tags     []string
	Question string
	Answer   string
}

type nativeFlashcardMetadata struct {
	ID   string   `yaml:"id"`
	Tags []string `yaml:"tags"`
}

type flashcardGrade string

const (
	flashcardGradeAgain flashcardGrade = "again"
	flashcardGradeHard  flashcardGrade = "hard"
	flashcardGradeGood  flashcardGrade = "good"
	flashcardGradeEasy  flashcardGrade = "easy"
)

type nativeFlashcardProgress struct {
	DueAt          time.Time      `json:"due_at,omitempty"`
	LastReviewedAt time.Time      `json:"last_reviewed_at,omitempty"`
	LastGrade      flashcardGrade `json:"last_grade,omitempty"`
	Interval       time.Duration  `json:"interval,omitempty"`
	Reviews        int            `json:"reviews,omitempty"`
	Lapses         int            `json:"lapses,omitempty"`
	Streak         int            `json:"streak,omitempty"`
}

type nativeFlashcardState struct {
	Cards map[string]nativeFlashcardProgress `json:"cards"`
}

type nativeFlashcardReviewSession struct {
	Deck      Snippet
	Cards     []nativeFlashcard
	Queue     []int
	Position  int
	Revealed  bool
	Completed int
}

func defaultNativeFlashcardDeckContent() string {
	return defaultNativeFlashcardDeckTemplate
}

func isNativeFlashcardDeckFile(file string) bool {
	name := filepath.Base(file)
	ext := strings.ToLower(filepath.Ext(name))
	if ext != ".md" {
		return false
	}
	return strings.TrimSuffix(name, ext) == nativeFlashcardDeckStem
}

func isNativeFlashcardDeck(snippet Snippet) bool {
	return isNativeFlashcardDeckFile(snippet.File)
}

func nativeFlashcardStatePath(home string, deck Snippet) (string, error) {
	deckPath, err := snippetStoragePath(home, deck)
	if err != nil {
		return "", err
	}
	base := strings.TrimSuffix(filepath.Base(deckPath), filepath.Ext(deckPath))
	return filepath.Join(filepath.Dir(deckPath), "."+base+".state.json"), nil
}

func readNativeFlashcardDeck(path string) (nativeFlashcardDeck, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nativeFlashcardDeck{}, err
	}
	return parseNativeFlashcardDeck(content)
}

func parseNativeFlashcardDeck(content []byte) (nativeFlashcardDeck, error) {
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	index := 0
	for index < len(lines) && strings.TrimSpace(lines[index]) == "" {
		index++
	}
	if index >= len(lines) || strings.TrimSpace(lines[index]) != nativeFlashcardHeaderLine {
		return nativeFlashcardDeck{}, fmt.Errorf("%w: missing %s header", errNativeFlashcardDeckInvalid, nativeFlashcardHeaderLine)
	}
	index++

	deck := nativeFlashcardDeck{Cards: make([]nativeFlashcard, 0, 4)}
	for index < len(lines) {
		for index < len(lines) && strings.TrimSpace(lines[index]) == "" {
			index++
		}
		if index >= len(lines) {
			break
		}
		if strings.TrimSpace(lines[index]) != "---" {
			return nativeFlashcardDeck{}, fmt.Errorf("%w: card block must start with ---", errNativeFlashcardDeckInvalid)
		}
		index++

		metaStart := index
		for index < len(lines) && strings.TrimSpace(lines[index]) != "---" {
			index++
		}
		if index >= len(lines) {
			return nativeFlashcardDeck{}, fmt.Errorf("%w: card metadata is not closed", errNativeFlashcardDeckInvalid)
		}

		var metadata nativeFlashcardMetadata
		if err := yaml.Unmarshal([]byte(strings.Join(lines[metaStart:index], "\n")), &metadata); err != nil {
			return nativeFlashcardDeck{}, fmt.Errorf("%w: %v", errNativeFlashcardDeckInvalid, err)
		}
		if strings.TrimSpace(metadata.ID) == "" {
			return nativeFlashcardDeck{}, fmt.Errorf("%w: card id is required", errNativeFlashcardDeckInvalid)
		}

		index++
		bodyStart := index
		inFence := false
		for index < len(lines) {
			trimmed := strings.TrimSpace(lines[index])
			if strings.HasPrefix(trimmed, "```") {
				inFence = !inFence
			}
			if !inFence && trimmed == "---" {
				break
			}
			index++
		}
		question, answer, err := parseNativeFlashcardBody(strings.Join(lines[bodyStart:index], "\n"))
		if err != nil {
			return nativeFlashcardDeck{}, err
		}
		deck.Cards = append(deck.Cards, nativeFlashcard{
			ID:       strings.TrimSpace(metadata.ID),
			Tags:     slices.Clone(metadata.Tags),
			Question: question,
			Answer:   answer,
		})
	}

	if len(deck.Cards) == 0 {
		return nativeFlashcardDeck{}, fmt.Errorf("%w: no cards found", errNativeFlashcardDeckInvalid)
	}
	return deck, nil
}

func parseNativeFlashcardBlock(block string) (nativeFlashcard, error) {
	trimmed := strings.TrimSpace(block)
	if trimmed == "" {
		return nativeFlashcard{}, fmt.Errorf("%w: empty block", errNativeFlashcardDeckInvalid)
	}
	deck, err := parseNativeFlashcardDeck([]byte(nativeFlashcardHeaderLine + "\n\n" + trimmed))
	if err != nil {
		return nativeFlashcard{}, err
	}
	if len(deck.Cards) != 1 {
		return nativeFlashcard{}, fmt.Errorf("%w: expected one card", errNativeFlashcardDeckInvalid)
	}
	return deck.Cards[0], nil
}

func parseNativeFlashcardBody(body string) (string, string, error) {
	lines := strings.Split(strings.TrimSpace(body), "\n")
	section := ""
	question := make([]string, 0, len(lines))
	answer := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "Q:"):
			section = "question"
			value := strings.TrimSpace(strings.TrimPrefix(trimmed, "Q:"))
			if value != "" {
				question = append(question, value)
			}
			continue
		case strings.HasPrefix(trimmed, "A:"):
			section = "answer"
			value := strings.TrimSpace(strings.TrimPrefix(trimmed, "A:"))
			if value != "" {
				answer = append(answer, value)
			}
			continue
		}

		switch section {
		case "question":
			question = append(question, line)
		case "answer":
			answer = append(answer, line)
		}
	}

	questionText := strings.TrimSpace(strings.Join(question, "\n"))
	answerText := strings.TrimSpace(strings.Join(answer, "\n"))
	if questionText == "" || answerText == "" {
		return "", "", fmt.Errorf("%w: each card needs Q: and A: sections", errNativeFlashcardDeckInvalid)
	}
	return questionText, answerText, nil
}

func readNativeFlashcardState(home string, deck Snippet) (nativeFlashcardState, error) {
	path, err := nativeFlashcardStatePath(home, deck)
	if err != nil {
		return nativeFlashcardState{}, err
	}
	content, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nativeFlashcardState{Cards: map[string]nativeFlashcardProgress{}}, nil
	}
	if err != nil {
		return nativeFlashcardState{}, err
	}

	state := nativeFlashcardState{}
	if err := json.Unmarshal(content, &state); err != nil {
		return nativeFlashcardState{}, err
	}
	if state.Cards == nil {
		state.Cards = map[string]nativeFlashcardProgress{}
	}
	return state, nil
}

func writeNativeFlashcardState(home string, deck Snippet, state nativeFlashcardState) error {
	if state.Cards == nil {
		state.Cards = map[string]nativeFlashcardProgress{}
	}
	path, err := nativeFlashcardStatePath(home, deck)
	if err != nil {
		return err
	}
	content, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}

func buildNativeFlashcardReviewSession(deck Snippet, parsed nativeFlashcardDeck, state nativeFlashcardState, now time.Time) (*nativeFlashcardReviewSession, error) {
	queue := make([]int, 0, len(parsed.Cards))
	for idx, card := range parsed.Cards {
		progress, ok := state.Cards[card.ID]
		if !ok || progress.DueAt.IsZero() || !progress.DueAt.After(now) {
			queue = append(queue, idx)
		}
	}
	if len(queue) == 0 {
		return nil, errNativeFlashcardNoCardsDue
	}
	return &nativeFlashcardReviewSession{
		Deck:  deck,
		Cards: slices.Clone(parsed.Cards),
		Queue: queue,
	}, nil
}

func scheduleNativeFlashcard(progress nativeFlashcardProgress, grade flashcardGrade, now time.Time) nativeFlashcardProgress {
	progress.LastReviewedAt = now
	progress.LastGrade = grade
	progress.Reviews++

	switch grade {
	case flashcardGradeAgain:
		progress.Interval = 10 * time.Minute
		progress.Lapses++
		progress.Streak = 0
	case flashcardGradeHard:
		if progress.Interval <= 0 {
			progress.Interval = 6 * time.Hour
		} else {
			progress.Interval = maxDuration(6*time.Hour, time.Duration(float64(progress.Interval)*1.4))
		}
		progress.Streak++
	case flashcardGradeEasy:
		if progress.Interval <= 0 {
			progress.Interval = 72 * time.Hour
		} else {
			progress.Interval = maxDuration(72*time.Hour, time.Duration(float64(progress.Interval)*3))
		}
		progress.Streak++
	default:
		if progress.Interval <= 0 {
			progress.Interval = 24 * time.Hour
		} else {
			progress.Interval = maxDuration(24*time.Hour, time.Duration(float64(progress.Interval)*2.2))
		}
		progress.Streak++
	}

	progress.DueAt = now.Add(progress.Interval)
	return progress
}

func maxDuration(left, right time.Duration) time.Duration {
	if left > right {
		return left
	}
	return right
}

func nativeFlashcardSummary(parsed nativeFlashcardDeck, state nativeFlashcardState, now time.Time) flashcardSummary {
	summary := flashcardSummary{
		rootCount: len(parsed.Cards),
	}
	for _, card := range parsed.Cards {
		progress, ok := state.Cards[card.ID]
		switch {
		case !ok || progress.DueAt.IsZero() || !progress.DueAt.After(now):
			summary.pendingCount++
		case progress.LastGrade == flashcardGradeAgain:
			summary.negativeCount++
		default:
			summary.positiveCount++
		}
	}
	return summary
}

func hasNativeFlashcardProgress(home string, deck Snippet) (bool, error) {
	state, err := readNativeFlashcardState(home, deck)
	if err != nil {
		return false, err
	}
	return len(state.Cards) > 0, nil
}

func resetNativeFlashcardProgressOnDisk(home string, decks []Snippet) ([]Snippet, error) {
	reset := make([]Snippet, 0, len(decks))
	for _, deck := range decks {
		if !isNativeFlashcardDeck(deck) {
			continue
		}
		hasProgress, err := hasNativeFlashcardProgress(home, deck)
		if err != nil {
			return nil, err
		}
		if !hasProgress {
			continue
		}
		path, err := nativeFlashcardStatePath(home, deck)
		if err != nil {
			return nil, err
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		reset = append(reset, deck)
	}
	return reset, nil
}

func nativeFlashcardIndicatorStates(home string, deck Snippet, now time.Time) []flashcardDeckState {
	path, err := snippetStoragePath(home, deck)
	if err != nil {
		return nil
	}
	parsed, err := readNativeFlashcardDeck(path)
	if err != nil {
		return nil
	}
	state, err := readNativeFlashcardState(home, deck)
	if err != nil {
		return nil
	}
	summary := nativeFlashcardSummary(parsed, state, now)
	states := make([]flashcardDeckState, 0, 2)
	if summary.positiveCount > 0 {
		states = append(states, flashcardDeckPositive)
	}
	if summary.negativeCount > 0 {
		states = append(states, flashcardDeckNegative)
	}
	return states
}

func (m *Model) startNativeFlashcardReview(deck Snippet) tea.Cmd {
	path, err := snippetStoragePath(m.config.Home, deck)
	if err != nil {
		m.displayError(err.Error())
		return nil
	}

	parsed, err := readNativeFlashcardDeck(path)
	if err != nil {
		m.displayError(err.Error())
		return nil
	}
	state, err := readNativeFlashcardState(m.config.Home, deck)
	if err != nil {
		m.displayError(err.Error())
		return nil
	}
	session, err := buildNativeFlashcardReviewSession(deck, parsed, state, time.Now())
	if err != nil {
		if errors.Is(err, errNativeFlashcardNoCardsDue) {
			m.displayError("No flashcards are due right now.")
			return nil
		}
		m.displayError(err.Error())
		return nil
	}

	m.flashcardSession = session
	m.state = reviewingFlashcardsState
	m.pane = contentPane
	m.updateKeyMap()
	return m.updateContent()
}

func (m *Model) stopNativeFlashcardReview() tea.Cmd {
	folder := m.selectedFolder()
	if m.flashcardSession != nil {
		folder = Folder(m.flashcardSession.Deck.Folder)
	}
	m.flashcardSession = nil
	m.state = navigatingState
	m.List().SetDelegate(snippetDelegate{styles: m.ListStyle, state: navigatingState, compact: m.isCollapsedPreview()})
	m.updateKeyMap()
	return m.updateFoldersForSelection(folder, true)
}

func (m *Model) gradeNativeFlashcard(grade flashcardGrade) tea.Cmd {
	if m.flashcardSession == nil || !m.flashcardSession.Revealed {
		return nil
	}

	deck := m.flashcardSession.Deck
	state, err := readNativeFlashcardState(m.config.Home, deck)
	if err != nil {
		m.displayError(err.Error())
		return nil
	}

	card := m.flashcardSession.Cards[m.flashcardSession.Queue[m.flashcardSession.Position]]
	progress := scheduleNativeFlashcard(state.Cards[card.ID], grade, time.Now())
	if state.Cards == nil {
		state.Cards = map[string]nativeFlashcardProgress{}
	}
	state.Cards[card.ID] = progress
	if err := writeNativeFlashcardState(m.config.Home, deck, state); err != nil {
		m.displayError(err.Error())
		return nil
	}

	m.flashcardSession.Completed++
	m.flashcardSession.Position++
	m.flashcardSession.Revealed = false
	if m.flashcardSession.Position >= len(m.flashcardSession.Queue) {
		return m.stopNativeFlashcardReview()
	}

	return m.updateContent()
}

func (m *Model) displayNativeFlashcardReview() {
	if m.flashcardSession == nil || len(m.flashcardSession.Queue) == 0 {
		m.displayError("Flashcard session unavailable.")
		return
	}

	cardIndex := m.flashcardSession.Queue[m.flashcardSession.Position]
	card := m.flashcardSession.Cards[cardIndex]
	lines := []string{
		m.ContentStyle.EmptyHint.Render("Nap flashcards"),
		"",
		m.ContentStyle.EmptyHint.Render(fmt.Sprintf("deck        %s", m.flashcardSession.Deck.Path())),
		m.ContentStyle.EmptyHint.Render(fmt.Sprintf("card        %d/%d", m.flashcardSession.Position+1, len(m.flashcardSession.Queue))),
	}
	if len(card.Tags) > 0 {
		lines = append(lines, m.ContentStyle.EmptyHint.Render(fmt.Sprintf("tags        %s", strings.Join(card.Tags, ", "))))
	}
	lines = append(lines, "", card.Question, "")
	if m.flashcardSession.Revealed {
		lines = append(lines,
			m.ContentStyle.EmptyHint.Render("answer"),
			"",
			card.Answer,
			"",
			fmt.Sprintf("%s %s", m.ContentStyle.EmptyHintKey.Render("1"), m.ContentStyle.EmptyHint.Render("• again")),
			fmt.Sprintf("%s %s", m.ContentStyle.EmptyHintKey.Render("2"), m.ContentStyle.EmptyHint.Render("• hard")),
			fmt.Sprintf("%s %s", m.ContentStyle.EmptyHintKey.Render("3"), m.ContentStyle.EmptyHint.Render("• good")),
			fmt.Sprintf("%s %s", m.ContentStyle.EmptyHintKey.Render("4"), m.ContentStyle.EmptyHint.Render("• easy")),
		)
	} else {
		lines = append(lines, fmt.Sprintf("%s %s", m.ContentStyle.EmptyHintKey.Render("space"), m.ContentStyle.EmptyHint.Render("• reveal answer")))
	}
	lines = append(lines, fmt.Sprintf("%s %s", m.ContentStyle.EmptyHintKey.Render("esc"), m.ContentStyle.EmptyHint.Render("• stop review")))

	m.LineNumbers.SetContent(strings.Repeat("  ~ \n", len(lines)))
	m.LineNumbers.SetYOffset(0)
	m.Code.SetContent(strings.Join(lines, "\n"))
	m.Code.SetYOffset(0)
}
