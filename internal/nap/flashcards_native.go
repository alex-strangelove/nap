package nap

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"gopkg.in/yaml.v3"
)

const (
	nativeFlashcardDeckStem     = "00-nap-cards"
	nativeFlashcardHeaderLineV1 = "<!-- nap-flashcards:v1 -->"
	nativeFlashcardHeaderLineV2 = "<!-- nap-deck: v2 -->"
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

type nativeFlashcardType string

const (
	nativeFlashcardTypeBasic         nativeFlashcardType = "basic"
	nativeFlashcardTypeSingleChoice  nativeFlashcardType = "single-choice"
	nativeFlashcardTypeMultiChoice   nativeFlashcardType = "multi-choice"
	nativeFlashcardTypeCodeCloze     nativeFlashcardType = "code-cloze"
	nativeFlashcardTypeOrderedRecall nativeFlashcardType = "ordered-recall"
	nativeFlashcardTypeTrace         nativeFlashcardType = "trace"
)

type nativeFlashcard struct {
	ID             string
	Type           nativeFlashcardType
	Tags           []string
	Question       string
	Trace          string
	Answer         string
	Explanation    string
	Options        []string
	CorrectOptions []string
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

type nativeFlashcardReviewPhase int

const (
	nativeFlashcardPhaseQuestion nativeFlashcardReviewPhase = iota
	nativeFlashcardPhaseResult
)

type nativeFlashcardReviewSession struct {
	Deck            Snippet
	Cards           []nativeFlashcard
	Queue           []int
	Position        int
	Completed       int
	Phase           nativeFlashcardReviewPhase
	Cursor          int
	Selections      map[int]bool
	DisplayOrder    []int
	OrderedSequence []int
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
	if index >= len(lines) {
		return nativeFlashcardDeck{}, fmt.Errorf("%w: missing deck header", errNativeFlashcardDeckInvalid)
	}

	switch strings.TrimSpace(lines[index]) {
	case nativeFlashcardHeaderLineV1:
		return parseNativeFlashcardDeckV1(lines[index+1:])
	case nativeFlashcardHeaderLineV2:
		return parseNativeFlashcardDeckV2(lines[index+1:])
	default:
		return nativeFlashcardDeck{}, fmt.Errorf("%w: missing supported deck header", errNativeFlashcardDeckInvalid)
	}
}

func parseNativeFlashcardDeckV1(lines []string) (nativeFlashcardDeck, error) {
	index := 0
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
		for index < len(lines) {
			trimmed := strings.TrimSpace(lines[index])
			if trimmed == "---" {
				break
			}
			index++
		}
		question, answer, err := parseNativeFlashcardBodyV1(strings.Join(lines[bodyStart:index], "\n"))
		if err != nil {
			return nativeFlashcardDeck{}, err
		}
		deck.Cards = append(deck.Cards, nativeFlashcard{
			ID:       strings.TrimSpace(metadata.ID),
			Type:     nativeFlashcardTypeBasic,
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

func parseNativeFlashcardDeckV2(lines []string) (nativeFlashcardDeck, error) {
	blocks := splitNativeFlashcardBlocks(strings.Join(lines, "\n"), "+++")
	deck := nativeFlashcardDeck{Cards: make([]nativeFlashcard, 0, len(blocks))}
	for _, block := range blocks {
		if strings.TrimSpace(block) == "" {
			continue
		}
		card, err := parseNativeFlashcardBlockV2(block)
		if err != nil {
			return nativeFlashcardDeck{}, err
		}
		deck.Cards = append(deck.Cards, card)
	}
	if len(deck.Cards) == 0 {
		return nativeFlashcardDeck{}, fmt.Errorf("%w: no cards found", errNativeFlashcardDeckInvalid)
	}
	return deck, nil
}

func splitNativeFlashcardBlocks(text, separator string) []string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	blocks := make([]string, 0, 4)
	current := make([]string, 0, len(lines))
	openFence := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if fence, ok := nativeFlashcardFenceDelimiter(trimmed); ok {
			if openFence == "" {
				openFence = fence
			} else if fence == openFence {
				openFence = ""
			}
		}
		if openFence == "" && trimmed == separator {
			blocks = append(blocks, strings.TrimSpace(strings.Join(current, "\n")))
			current = current[:0]
			continue
		}
		current = append(current, line)
	}
	if block := strings.TrimSpace(strings.Join(current, "\n")); block != "" {
		blocks = append(blocks, block)
	}
	return blocks
}

func nativeFlashcardFenceDelimiter(line string) (string, bool) {
	if !strings.HasPrefix(line, "`") {
		return "", false
	}
	count := 0
	for count < len(line) && line[count] == '`' {
		count++
	}
	if count < 3 {
		return "", false
	}
	return line[:count], true
}

func parseNativeFlashcardBlock(block string) (nativeFlashcard, error) {
	trimmed := strings.TrimSpace(block)
	if trimmed == "" {
		return nativeFlashcard{}, fmt.Errorf("%w: empty block", errNativeFlashcardDeckInvalid)
	}
	deck, err := parseNativeFlashcardDeck([]byte(nativeFlashcardHeaderLineV2 + "\n\n" + trimmed))
	if err != nil {
		return nativeFlashcard{}, err
	}
	if len(deck.Cards) != 1 {
		return nativeFlashcard{}, fmt.Errorf("%w: expected one card", errNativeFlashcardDeckInvalid)
	}
	return deck.Cards[0], nil
}

func parseNativeFlashcardBlockV2(block string) (nativeFlashcard, error) {
	lines := strings.Split(strings.TrimSpace(block), "\n")
	card := nativeFlashcard{
		Type:        nativeFlashcardTypeBasic,
		Tags:        []string{},
		Options:     []string{},
		Explanation: "",
	}

	index := 0
	for index < len(lines) {
		trimmed := strings.TrimSpace(lines[index])
		if trimmed == "" {
			index++
			continue
		}
		key, value, ok := parseNativeFlashcardCommentMetadata(trimmed)
		if !ok {
			break
		}
		switch key {
		case "id":
			card.ID = value
		case "type":
			card.Type = nativeFlashcardType(value)
		}
		index++
	}

	if card.ID == "" {
		return nativeFlashcard{}, fmt.Errorf("%w: card id is required", errNativeFlashcardDeckInvalid)
	}
	if !card.Type.valid() {
		return nativeFlashcard{}, fmt.Errorf("%w: unsupported card type %q", errNativeFlashcardDeckInvalid, card.Type)
	}

	sections, err := parseNativeFlashcardSections(lines[index:])
	if err != nil {
		return nativeFlashcard{}, err
	}

	card.Question = strings.TrimSpace(sections["Prompt"])
	card.Trace = strings.TrimSpace(sections["Trace"])
	card.Answer = strings.TrimSpace(sections["Answer"])
	card.Explanation = strings.TrimSpace(sections["Explanation"])
	card.Options = parseNativeFlashcardListSection(sections["Options"])
	card.CorrectOptions = parseNativeFlashcardListSection(sections["Correct"])
	if card.Type != nativeFlashcardTypeOrderedRecall && card.Type != nativeFlashcardTypeTrace && len(card.CorrectOptions) == 0 && card.Answer != "" {
		card.CorrectOptions = []string{card.Answer}
	}
	if tagsText := strings.TrimSpace(sections["Tags"]); tagsText != "" {
		card.Tags = parseNativeFlashcardTags(tagsText)
	}

	if card.Question == "" {
		return nativeFlashcard{}, fmt.Errorf("%w: each v2 card needs a Prompt section", errNativeFlashcardDeckInvalid)
	}
	switch card.Type {
	case nativeFlashcardTypeBasic:
		if card.Answer == "" {
			return nativeFlashcard{}, fmt.Errorf("%w: basic cards need an Answer section", errNativeFlashcardDeckInvalid)
		}
	case nativeFlashcardTypeSingleChoice, nativeFlashcardTypeCodeCloze:
		if len(card.Options) < 2 || len(card.CorrectOptions) != 1 {
			return nativeFlashcard{}, fmt.Errorf("%w: %s cards need options and exactly one correct answer", errNativeFlashcardDeckInvalid, card.Type)
		}
	case nativeFlashcardTypeMultiChoice:
		if len(card.Options) < 2 || len(card.CorrectOptions) == 0 {
			return nativeFlashcard{}, fmt.Errorf("%w: multi-choice cards need options and at least one correct answer", errNativeFlashcardDeckInvalid)
		}
	case nativeFlashcardTypeOrderedRecall:
		if len(card.Options) < 2 {
			return nativeFlashcard{}, fmt.Errorf("%w: ordered-recall cards need at least two ordered steps", errNativeFlashcardDeckInvalid)
		}
		if card.Answer != "" || len(card.CorrectOptions) > 0 {
			return nativeFlashcard{}, fmt.Errorf("%w: ordered-recall cards use the Options section as the canonical order", errNativeFlashcardDeckInvalid)
		}
		card.CorrectOptions = slices.Clone(card.Options)
	case nativeFlashcardTypeTrace:
		if card.Trace == "" {
			return nativeFlashcard{}, fmt.Errorf("%w: trace cards need a Trace section", errNativeFlashcardDeckInvalid)
		}
		if len(card.Options) > 0 {
			if card.Answer != "" {
				return nativeFlashcard{}, fmt.Errorf("%w: trace cards must use either options/correct or answer, not both", errNativeFlashcardDeckInvalid)
			}
			if len(card.Options) < 2 || len(card.CorrectOptions) != 1 {
				return nativeFlashcard{}, fmt.Errorf("%w: trace cards with options need exactly one correct answer", errNativeFlashcardDeckInvalid)
			}
		} else {
			if card.Answer == "" || len(card.CorrectOptions) > 0 {
				return nativeFlashcard{}, fmt.Errorf("%w: reveal-only trace cards need an Answer section and no Correct section", errNativeFlashcardDeckInvalid)
			}
		}
	}

	return card, nil
}

func parseNativeFlashcardCommentMetadata(line string) (string, string, bool) {
	if !strings.HasPrefix(line, "<!--") || !strings.HasSuffix(line, "-->") {
		return "", "", false
	}
	value := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "<!--"), "-->"))
	parts := strings.SplitN(value, ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
}

func parseNativeFlashcardSections(lines []string) (map[string]string, error) {
	sections := map[string]string{}
	current := ""
	buffer := []string{}
	store := func() {
		if current == "" {
			return
		}
		sections[current] = strings.TrimSpace(strings.Join(buffer, "\n"))
		buffer = buffer[:0]
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch trimmed {
		case "Prompt:", "Trace:", "Options:", "Answer:", "Correct:", "Explanation:", "Tags:":
			store()
			current = strings.TrimSuffix(trimmed, ":")
			continue
		}
		if current == "" {
			if trimmed == "" {
				continue
			}
			return nil, fmt.Errorf("%w: unexpected content before first section", errNativeFlashcardDeckInvalid)
		}
		buffer = append(buffer, line)
	}
	store()
	return sections, nil
}

func parseNativeFlashcardListSection(section string) []string {
	if strings.TrimSpace(section) == "" {
		return nil
	}
	lines := strings.Split(section, "\n")
	values := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "- ") {
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
		}
		values = append(values, trimmed)
	}
	return values
}

func parseNativeFlashcardTags(section string) []string {
	parts := strings.Split(section, ",")
	tags := make([]string, 0, len(parts))
	for _, part := range parts {
		tag := strings.TrimSpace(strings.TrimPrefix(part, "-"))
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

func parseNativeFlashcardBodyV1(body string) (string, string, error) {
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
	session := &nativeFlashcardReviewSession{
		Deck:       deck,
		Cards:      slices.Clone(parsed.Cards),
		Queue:      queue,
		Phase:      nativeFlashcardPhaseQuestion,
		Selections: map[int]bool{},
	}
	session.prepareCurrentCard()
	return session, nil
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
		case progress.LastGrade == flashcardGradeHard:
			summary.recallCount++
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
	states := make([]flashcardDeckState, 0, 3)
	hasRecall := false
	hasPositive := false
	hasNegative := false
	for _, card := range parsed.Cards {
		progress, ok := state.Cards[card.ID]
		if !ok || progress.DueAt.IsZero() || !progress.DueAt.After(now) {
			continue
		}
		switch progress.LastGrade {
		case flashcardGradeAgain:
			hasNegative = true
		case flashcardGradeHard:
			hasRecall = true
		default:
			hasPositive = true
		}
	}
	if hasRecall {
		states = append(states, flashcardDeckRecall)
	}
	if hasPositive {
		states = append(states, flashcardDeckPositive)
	}
	if hasNegative || summary.negativeCount > 0 {
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
	if m.flashcardSession == nil || m.flashcardSession.Phase != nativeFlashcardPhaseResult {
		return nil
	}

	deck := m.flashcardSession.Deck
	state, err := readNativeFlashcardState(m.config.Home, deck)
	if err != nil {
		m.displayError(err.Error())
		return nil
	}

	card := m.flashcardSession.currentCard()
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
	if m.flashcardSession.Position >= len(m.flashcardSession.Queue) {
		return m.stopNativeFlashcardReview()
	}
	m.flashcardSession.prepareCurrentCard()

	return m.updateContent()
}

func (m *Model) moveNativeFlashcardSelection(delta int) tea.Cmd {
	if m.flashcardSession == nil {
		return nil
	}
	card := m.flashcardSession.currentCard()
	if !card.hasOptions() || m.flashcardSession.Phase != nativeFlashcardPhaseQuestion {
		return nil
	}
	optionCount := len(card.Options)
	if optionCount == 0 {
		return nil
	}
	positions := m.flashcardSession.selectableCursorPositions(card)
	if len(positions) == 0 {
		return nil
	}
	current := 0
	for idx, position := range positions {
		if position == m.flashcardSession.Cursor {
			current = idx
			break
		}
	}
	m.flashcardSession.Cursor = positions[(current+delta+len(positions))%len(positions)]
	return m.updateContent()
}

func (m *Model) toggleNativeFlashcardSelection() tea.Cmd {
	if m.flashcardSession == nil {
		return nil
	}
	card := m.flashcardSession.currentCard()
	if card.Type != nativeFlashcardTypeMultiChoice || m.flashcardSession.Phase != nativeFlashcardPhaseQuestion {
		return nil
	}
	if m.flashcardSession.Selections == nil {
		m.flashcardSession.Selections = map[int]bool{}
	}
	index, ok := m.flashcardSession.optionIndexAtCursor(card)
	if !ok {
		return nil
	}
	if m.flashcardSession.Selections[index] {
		delete(m.flashcardSession.Selections, index)
	} else {
		m.flashcardSession.Selections[index] = true
	}
	return m.updateContent()
}

func (m *Model) submitNativeFlashcardAnswer() tea.Cmd {
	if m.flashcardSession == nil || m.flashcardSession.Phase != nativeFlashcardPhaseQuestion {
		return nil
	}
	card := m.flashcardSession.currentCard()
	switch card.Type {
	case nativeFlashcardTypeBasic:
		m.flashcardSession.Phase = nativeFlashcardPhaseResult
	case nativeFlashcardTypeSingleChoice, nativeFlashcardTypeCodeCloze:
		index, ok := m.flashcardSession.optionIndexAtCursor(card)
		if !ok {
			return nil
		}
		m.flashcardSession.Selections = map[int]bool{index: true}
		m.flashcardSession.Phase = nativeFlashcardPhaseResult
	case nativeFlashcardTypeMultiChoice:
		if len(m.flashcardSession.Selections) == 0 {
			return nil
		}
		m.flashcardSession.Phase = nativeFlashcardPhaseResult
	case nativeFlashcardTypeOrderedRecall:
		if !m.flashcardSession.orderedRecallComplete(card) {
			return m.appendNativeFlashcardOrderedStep()
		}
		m.flashcardSession.Phase = nativeFlashcardPhaseResult
	case nativeFlashcardTypeTrace:
		if len(card.Options) == 0 {
			m.flashcardSession.Phase = nativeFlashcardPhaseResult
			return m.updateContent()
		}
		index, ok := m.flashcardSession.optionIndexAtCursor(card)
		if !ok {
			return nil
		}
		m.flashcardSession.Selections = map[int]bool{index: true}
		m.flashcardSession.Phase = nativeFlashcardPhaseResult
	default:
		return nil
	}
	return m.updateContent()
}

func (m *Model) appendNativeFlashcardOrderedStep() tea.Cmd {
	if m.flashcardSession == nil || m.flashcardSession.Phase != nativeFlashcardPhaseQuestion {
		return nil
	}
	card := m.flashcardSession.currentCard()
	if card.Type != nativeFlashcardTypeOrderedRecall {
		return nil
	}
	if m.flashcardSession.orderedRecallComplete(card) {
		return nil
	}
	index, ok := m.flashcardSession.optionIndexAtCursor(card)
	if !ok || m.flashcardSession.selected(index) {
		return nil
	}
	if m.flashcardSession.Selections == nil {
		m.flashcardSession.Selections = map[int]bool{}
	}
	m.flashcardSession.Selections[index] = true
	m.flashcardSession.OrderedSequence = append(m.flashcardSession.OrderedSequence, index)
	if next, ok := m.flashcardSession.firstSelectableCursorPosition(card); ok {
		m.flashcardSession.Cursor = next
	}
	return m.updateContent()
}

func (m *Model) removeNativeFlashcardOrderedStep() tea.Cmd {
	if m.flashcardSession == nil || m.flashcardSession.Phase != nativeFlashcardPhaseQuestion {
		return nil
	}
	card := m.flashcardSession.currentCard()
	if card.Type != nativeFlashcardTypeOrderedRecall || len(m.flashcardSession.OrderedSequence) == 0 {
		return nil
	}
	last := m.flashcardSession.OrderedSequence[len(m.flashcardSession.OrderedSequence)-1]
	m.flashcardSession.OrderedSequence = m.flashcardSession.OrderedSequence[:len(m.flashcardSession.OrderedSequence)-1]
	delete(m.flashcardSession.Selections, last)
	for position, index := range m.flashcardSession.displayOrder(card) {
		if index == last {
			m.flashcardSession.Cursor = position
			break
		}
	}
	return m.updateContent()
}

func (m *Model) displayNativeFlashcardReview() {
	if m.flashcardSession == nil || len(m.flashcardSession.Queue) == 0 {
		m.displayError("Flashcard session unavailable.")
		return
	}

	card := m.flashcardSession.currentCard()
	lines := []string{
		m.ContentStyle.EmptyHint.Render("Napcards review"),
		"",
		m.ContentStyle.EmptyHint.Render(fmt.Sprintf("deck        %s", m.flashcardSession.Deck.Path())),
		m.ContentStyle.EmptyHint.Render(fmt.Sprintf("card        %d/%d", m.flashcardSession.Position+1, len(m.flashcardSession.Queue))),
		m.ContentStyle.EmptyHint.Render(fmt.Sprintf("type        %s", card.Type)),
	}
	if len(card.Tags) > 0 {
		lines = append(lines, m.ContentStyle.EmptyHint.Render(fmt.Sprintf("tags        %s", strings.Join(card.Tags, ", "))))
	}
	lines = append(lines, "", card.Question, "")
	lines = append(lines, m.renderNativeFlashcardTrace(card)...)
	lines = append(lines, m.renderNativeFlashcardOptions(card)...)
	lines = append(lines, m.renderNativeFlashcardResult(card)...)

	for _, hint := range m.nativeFlashcardHints(card) {
		lines = append(lines, fmt.Sprintf("%s %s", m.ContentStyle.EmptyHintKey.Render(hint.binding.Help().Key), m.ContentStyle.EmptyHint.Render("• "+hint.help)))
	}

	m.LineNumbers.SetContent(strings.Repeat("  ~ \n", len(lines)))
	m.LineNumbers.SetYOffset(0)
	m.Code.SetContent(strings.Join(lines, "\n"))
	m.Code.SetYOffset(0)
}

func (m *Model) renderNativeFlashcardOptions(card nativeFlashcard) []string {
	if !card.hasOptions() {
		return nil
	}
	if card.Type == nativeFlashcardTypeOrderedRecall {
		return m.renderNativeFlashcardOrderedRecallOptions(card)
	}
	lines := []string{m.ContentStyle.EmptyHint.Render("options"), ""}
	for position, idx := range m.flashcardSession.displayOrder(card) {
		option := card.Options[idx]
		prefix := "  "
		if m.flashcardSession.Phase == nativeFlashcardPhaseQuestion && position == m.flashcardSession.Cursor {
			prefix = "> "
		}
		selection := "[ ]"
		if m.flashcardSession.selected(idx) {
			selection = "[x]"
		}
		switch {
		case m.flashcardSession.Phase != nativeFlashcardPhaseResult:
		case card.optionIsCorrect(option) && m.flashcardSession.selected(idx):
			selection = "[✓]"
		case card.optionIsCorrect(option):
			selection = "[+]"
		case m.flashcardSession.selected(idx):
			selection = "[x]"
		}
		lines = append(lines, fmt.Sprintf("%s%s %s", prefix, selection, option))
	}
	lines = append(lines, "")
	return lines
}

func (m *Model) renderNativeFlashcardTrace(card nativeFlashcard) []string {
	if card.Type != nativeFlashcardTypeTrace || strings.TrimSpace(card.Trace) == "" {
		return nil
	}
	return []string{
		m.ContentStyle.EmptyHint.Render("trace"),
		"",
		card.Trace,
		"",
	}
}

func (m *Model) renderNativeFlashcardOrderedRecallOptions(card nativeFlashcard) []string {
	if m.flashcardSession.Phase == nativeFlashcardPhaseResult {
		return nil
	}
	lines := []string{m.ContentStyle.EmptyHint.Render("build order"), ""}
	selected := m.flashcardSession.selectedOptionTexts(card)
	if len(selected) == 0 {
		lines = append(lines, m.ContentStyle.EmptyHint.Render("  no steps chosen yet"))
	} else {
		lines = append(lines, numberedFlashcardLines(selected)...)
	}
	lines = append(lines, "", m.ContentStyle.EmptyHint.Render("remaining steps"), "")
	remaining := 0
	for position, idx := range m.flashcardSession.displayOrder(card) {
		if m.flashcardSession.selected(idx) {
			continue
		}
		remaining++
		prefix := "  "
		if m.flashcardSession.Phase == nativeFlashcardPhaseQuestion && position == m.flashcardSession.Cursor {
			prefix = "> "
		}
		lines = append(lines, fmt.Sprintf("%s[ ] %s", prefix, card.Options[idx]))
	}
	if remaining == 0 {
		lines = append(lines, m.ContentStyle.EmptyHint.Render("  all steps chosen - press enter to submit"))
	}
	lines = append(lines, "")
	return lines
}

func (m *Model) renderNativeFlashcardResult(card nativeFlashcard) []string {
	if m.flashcardSession.Phase != nativeFlashcardPhaseResult {
		return nil
	}
	label := m.nativeFlashcardResultLabel(card)
	lines := []string{
		m.ContentStyle.EmptyHint.Render("result      ") + m.renderNativeFlashcardResultLabel(label),
	}
	if card.Type == nativeFlashcardTypeOrderedRecall {
		lines = append(lines,
			"",
			m.ContentStyle.EmptyHint.Render("order check"),
			"",
		)
		lines = append(lines, m.renderNativeFlashcardOrderedRecallCheck(card)...)
	} else if card.Answer != "" {
		lines = append(lines, "", m.ContentStyle.EmptyHint.Render("answer"), "", card.Answer)
	} else if len(card.CorrectOptions) > 0 {
		lines = append(lines, "", m.ContentStyle.EmptyHint.Render("correct"), "", strings.Join(card.CorrectOptions, "\n"))
	}
	if card.Explanation != "" {
		lines = append(lines, "", m.ContentStyle.EmptyHint.Render("explanation"), "", card.Explanation)
	}
	lines = append(lines, "",
		fmt.Sprintf("%s %s", m.ContentStyle.EmptyHintKey.Render("1"), m.ContentStyle.EmptyHint.Render("• again")),
		fmt.Sprintf("%s %s", m.ContentStyle.EmptyHintKey.Render("2"), m.ContentStyle.EmptyHint.Render("• hard")),
		fmt.Sprintf("%s %s", m.ContentStyle.EmptyHintKey.Render("3"), m.ContentStyle.EmptyHint.Render("• good")),
		fmt.Sprintf("%s %s", m.ContentStyle.EmptyHintKey.Render("4"), m.ContentStyle.EmptyHint.Render("• easy")),
	)
	return lines
}

func (m *Model) renderNativeFlashcardOrderedRecallCheck(card nativeFlashcard) []string {
	selectedPositions := m.flashcardSession.orderedRecallPositions()
	lines := make([]string, 0, len(card.CorrectOptions))
	for idx, step := range card.CorrectOptions {
		status := "[✓]"
		suffix := ""
		if selectedAt, ok := selectedPositions[step]; ok && selectedAt != idx {
			status = "[x]"
			suffix = fmt.Sprintf(" (you put it at %d)", selectedAt+1)
		}
		lines = append(lines, fmt.Sprintf("%s %d. %s%s", status, idx+1, step, suffix))
	}
	return lines
}

func (m *Model) renderNativeFlashcardResultLabel(label string) string {
	switch label {
	case "correct":
		return m.ContentStyle.FlashcardPositive.Render(label)
	case "incorrect":
		return m.ContentStyle.FlashcardNegative.Render(label)
	default:
		return m.ContentStyle.FlashcardPending.Render(label)
	}
}

func (m *Model) nativeFlashcardResultLabel(card nativeFlashcard) string {
	if card.Type == nativeFlashcardTypeBasic || (card.Type == nativeFlashcardTypeTrace && len(card.Options) == 0) {
		return "revealed"
	}
	if card.Type == nativeFlashcardTypeOrderedRecall {
		if card.isCorrectOrderedSelection(m.flashcardSession.selectedOptionTexts(card)) {
			return "correct"
		}
		return "incorrect"
	}
	if card.isCorrectSelection(m.flashcardSession.selectedOptionTexts(card)) {
		return "correct"
	}
	return "incorrect"
}

func (m *Model) nativeFlashcardHints(card nativeFlashcard) []keyHint {
	switch m.flashcardSession.Phase {
	case nativeFlashcardPhaseQuestion:
		switch card.Type {
		case nativeFlashcardTypeBasic:
			return []keyHint{
				{binding: keyBinding("space", "reveal answer"), help: "reveal answer"},
				{binding: keyBinding("esc", "stop review"), help: "stop review"},
			}
		case nativeFlashcardTypeMultiChoice:
			return []keyHint{
				{binding: keyBinding("↑/↓", "move"), help: "move"},
				{binding: keyBinding("space", "toggle option"), help: "toggle option"},
				{binding: keyBinding("enter", "submit answer"), help: "submit answer"},
				{binding: keyBinding("esc", "stop review"), help: "stop review"},
			}
		case nativeFlashcardTypeOrderedRecall:
			action := "add step"
			if m.flashcardSession.orderedRecallComplete(card) {
				action = "submit order"
			}
			return []keyHint{
				{binding: keyBinding("↑/↓", "move"), help: "move"},
				{binding: keyBinding("enter/space", action), help: action},
				{binding: keyBinding("backspace", "remove last step"), help: "remove last step"},
				{binding: keyBinding("esc", "stop review"), help: "stop review"},
			}
		case nativeFlashcardTypeTrace:
			if len(card.Options) == 0 {
				return []keyHint{
					{binding: keyBinding("space/enter", "reveal answer"), help: "reveal answer"},
					{binding: keyBinding("esc", "stop review"), help: "stop review"},
				}
			}
			return []keyHint{
				{binding: keyBinding("↑/↓", "move"), help: "move"},
				{binding: keyBinding("enter", "submit answer"), help: "submit answer"},
				{binding: keyBinding("esc", "stop review"), help: "stop review"},
			}
		default:
			return []keyHint{
				{binding: keyBinding("↑/↓", "move"), help: "move"},
				{binding: keyBinding("enter", "submit answer"), help: "submit answer"},
				{binding: keyBinding("esc", "stop review"), help: "stop review"},
			}
		}
	default:
		return []keyHint{
			{binding: keyBinding("esc", "stop review"), help: "stop review"},
		}
	}
}

func keyBinding(keyName, helpText string) key.Binding {
	return key.NewBinding(key.WithKeys(keyName), key.WithHelp(keyName, helpText))
}

func (t nativeFlashcardType) valid() bool {
	switch t {
	case nativeFlashcardTypeBasic, nativeFlashcardTypeSingleChoice, nativeFlashcardTypeMultiChoice, nativeFlashcardTypeCodeCloze, nativeFlashcardTypeOrderedRecall, nativeFlashcardTypeTrace:
		return true
	default:
		return false
	}
}

func (c nativeFlashcard) hasOptions() bool {
	return len(c.Options) > 0
}

func (c nativeFlashcard) optionIsCorrect(option string) bool {
	for _, answer := range c.CorrectOptions {
		if strings.TrimSpace(answer) == strings.TrimSpace(option) {
			return true
		}
	}
	return false
}

func (c nativeFlashcard) isCorrectSelection(selected []string) bool {
	if len(selected) != len(c.CorrectOptions) {
		return false
	}
	want := make([]string, 0, len(c.CorrectOptions))
	for _, answer := range c.CorrectOptions {
		want = append(want, strings.TrimSpace(answer))
	}
	got := make([]string, 0, len(selected))
	for _, answer := range selected {
		got = append(got, strings.TrimSpace(answer))
	}
	slices.Sort(want)
	slices.Sort(got)
	return slices.Equal(want, got)
}

func (c nativeFlashcard) isCorrectOrderedSelection(selected []string) bool {
	if len(selected) != len(c.CorrectOptions) {
		return false
	}
	for idx, answer := range c.CorrectOptions {
		if strings.TrimSpace(answer) != strings.TrimSpace(selected[idx]) {
			return false
		}
	}
	return true
}

func numberedFlashcardLines(values []string) []string {
	lines := make([]string, 0, len(values))
	for idx, value := range values {
		lines = append(lines, fmt.Sprintf(" %d. %s", idx+1, value))
	}
	return lines
}

func (s *nativeFlashcardReviewSession) currentCard() nativeFlashcard {
	return s.Cards[s.Queue[s.Position]]
}

func (s *nativeFlashcardReviewSession) prepareCurrentCard() {
	if len(s.Queue) == 0 || s.Position >= len(s.Queue) {
		return
	}
	card := s.currentCard()
	s.Phase = nativeFlashcardPhaseQuestion
	s.Selections = map[int]bool{}
	s.OrderedSequence = nil
	s.DisplayOrder = nativeFlashcardDisplayOrder(card)
	if cursor, ok := s.firstSelectableCursorPosition(card); ok {
		s.Cursor = cursor
	} else {
		s.Cursor = 0
	}
}

func (s *nativeFlashcardReviewSession) selected(index int) bool {
	if s.Selections == nil {
		return false
	}
	return s.Selections[index]
}

func (s *nativeFlashcardReviewSession) selectedOptionTexts(card nativeFlashcard) []string {
	if card.Type == nativeFlashcardTypeOrderedRecall {
		if len(s.OrderedSequence) == 0 {
			return nil
		}
		answers := make([]string, 0, len(s.OrderedSequence))
		for _, idx := range s.OrderedSequence {
			if idx >= 0 && idx < len(card.Options) {
				answers = append(answers, card.Options[idx])
			}
		}
		return answers
	}
	if len(s.Selections) == 0 {
		return nil
	}
	answers := make([]string, 0, len(s.Selections))
	for idx := range card.Options {
		if s.selected(idx) {
			answers = append(answers, card.Options[idx])
		}
	}
	return answers
}

func (s *nativeFlashcardReviewSession) displayOrder(card nativeFlashcard) []int {
	if len(s.DisplayOrder) == len(card.Options) {
		return s.DisplayOrder
	}
	order := make([]int, len(card.Options))
	for idx := range order {
		order[idx] = idx
	}
	return order
}

func (s *nativeFlashcardReviewSession) optionIndexAtCursor(card nativeFlashcard) (int, bool) {
	order := s.displayOrder(card)
	if s.Cursor < 0 || s.Cursor >= len(order) {
		return 0, false
	}
	return order[s.Cursor], true
}

func (s *nativeFlashcardReviewSession) selectableCursorPositions(card nativeFlashcard) []int {
	order := s.displayOrder(card)
	positions := make([]int, 0, len(order))
	for position, idx := range order {
		if card.Type == nativeFlashcardTypeOrderedRecall && s.selected(idx) {
			continue
		}
		positions = append(positions, position)
	}
	return positions
}

func (s *nativeFlashcardReviewSession) firstSelectableCursorPosition(card nativeFlashcard) (int, bool) {
	positions := s.selectableCursorPositions(card)
	if len(positions) == 0 {
		return 0, false
	}
	return positions[0], true
}

func (s *nativeFlashcardReviewSession) orderedRecallComplete(card nativeFlashcard) bool {
	return card.Type == nativeFlashcardTypeOrderedRecall && len(s.OrderedSequence) == len(card.Options)
}

func (s *nativeFlashcardReviewSession) orderedRecallPositions() map[string]int {
	positions := make(map[string]int, len(s.OrderedSequence))
	card := s.currentCard()
	for pos, idx := range s.OrderedSequence {
		if idx >= 0 && idx < len(card.Options) {
			positions[card.Options[idx]] = pos
		}
	}
	return positions
}

func nativeFlashcardDisplayOrder(card nativeFlashcard) []int {
	order := make([]int, len(card.Options))
	for idx := range order {
		order[idx] = idx
	}
	if card.Type != nativeFlashcardTypeOrderedRecall || len(order) < 2 {
		return order
	}
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(card.ID))
	source := rand.NewSource(int64(hasher.Sum64()))
	shuffled := rand.New(source)
	shuffled.Shuffle(len(order), func(i, j int) {
		order[i], order[j] = order[j], order[i]
	})
	identity := true
	for idx, value := range order {
		if idx != value {
			identity = false
			break
		}
	}
	if identity {
		order[0], order[1] = order[1], order[0]
	}
	return order
}
