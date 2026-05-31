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

func TestCreateFlashcardDeckKeepsSelectedSnippetForDrafting(t *testing.T) {
	tmp := tmpHome(t)
	m := newTestModel()
	m.config.Home = tmp
	m.config.FlashcardsEnabled = true
	source := Snippet{
		Name:     "boot-walk",
		Folder:   defaultSnippetFolder,
		File:     "boot-walk.go",
		Language: "go",
		Date:     time.Now(),
	}
	if err := os.MkdirAll(filepath.Join(tmp, defaultSnippetFolder), 0o755); err != nil {
		t.Fatalf("could not create snippet folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, defaultSnippetFolder, source.File), []byte("fmt.Println(\"boot\")\n"), 0o644); err != nil {
		t.Fatalf("could not write source snippet: %v", err)
	}
	m.Lists[Folder(defaultSnippetFolder)] = newList([]list.Item{source}, 20, m.ListStyle)
	m.Folders.SetItems([]list.Item{source})
	m.Folders.Select(0)
	m.selectSnippetInFolder(Folder(defaultSnippetFolder), source)
	m.updateKeyMap()

	got := runModelCmd(m, m.createFlashcardDeck())
	if selected := got.selectedSnippet(); selected.Path() != source.Path() {
		t.Fatalf("expected selected snippet to stay on source after deck creation, got %#v", selected)
	}
	if !got.keys.DraftFlashcard.Enabled() {
		t.Fatal("expected draft flashcard key to stay enabled after deck creation")
	}
}

func TestCreateFlashcardDeckClearsStaleNativeState(t *testing.T) {
	tmp := tmpHome(t)
	m := newTestModel()
	m.config.Home = tmp
	m.config.FlashcardsEnabled = true
	m.updateKeyMap()

	deck := Snippet{
		Name:     nativeFlashcardDeckStem,
		Date:     time.Now(),
		File:     nativeFlashcardDeckStem + ".md",
		Language: "md",
		Folder:   defaultSnippetFolder,
	}
	if err := os.MkdirAll(filepath.Join(tmp, defaultSnippetFolder), 0o755); err != nil {
		t.Fatalf("could not create deck folder: %v", err)
	}
	if err := writeNativeFlashcardState(tmp, deck, nativeFlashcardState{
		Cards: map[string]nativeFlashcardProgress{
			"linux-paging-4level-walk": {Reviews: 1, LastGrade: flashcardGradeGood, DueAt: time.Now().Add(24 * time.Hour)},
		},
	}); err != nil {
		t.Fatalf("could not seed stale native state: %v", err)
	}

	if msg := m.createFlashcardDeck()(); msg == nil {
		t.Fatal("expected create flashcard deck message")
	}

	statePath, err := nativeFlashcardStatePath(tmp, deck)
	if err != nil {
		t.Fatalf("could not resolve native state path: %v", err)
	}
	if _, err := os.Stat(statePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected stale native state to be cleared on deck creation, got err=%v", err)
	}
}

func TestNativeFlashcardDraftBlockCreatesValidBasicCard(t *testing.T) {
	source := Snippet{
		Name:     "boot-walk",
		Folder:   defaultSnippetFolder,
		File:     "boot-walk.go",
		Language: "go",
	}

	block := nativeFlashcardDraftBlock(source, defaultNativeFlashcardDeckContent(), "Answer:\nfmt.Println(\"boot\")\n")
	card, err := parseNativeFlashcardBlock(block)
	if err != nil {
		t.Fatalf("expected draft block to parse, got %v", err)
	}
	if card.ID != "draft-boot-walk" {
		t.Fatalf("unexpected draft id: got %q", card.ID)
	}
	if card.Type != nativeFlashcardTypeBasic {
		t.Fatalf("expected basic draft card, got %q", card.Type)
	}
	if !strings.Contains(card.Answer, "TODO: replace this with the key fact") {
		t.Fatalf("expected draft answer placeholder, got %q", card.Answer)
	}
	if !strings.Contains(card.Answer, `\Answer:`) {
		t.Fatalf("expected section-like snippet line to be escaped, got %q", card.Answer)
	}
	if !strings.Contains(card.Explanation, source.Path()) {
		t.Fatalf("expected source path in explanation, got %q", card.Explanation)
	}
}

func TestNativeFlashcardDraftBlockUsesUniqueIDSuffix(t *testing.T) {
	source := Snippet{
		Name:     "boot-walk",
		Folder:   defaultSnippetFolder,
		File:     "boot-walk.go",
		Language: "go",
	}

	existing := defaultNativeFlashcardDeckContent() + `

+++

<!-- id: draft-boot-walk -->
<!-- type: basic -->

Prompt:
Existing draft.

Answer:
Keep me.
`
	block := nativeFlashcardDraftBlock(source, existing, "fmt.Println(\"boot\")\n")
	if !strings.Contains(block, "<!-- id: draft-boot-walk-2 -->") {
		t.Fatalf("expected duplicate draft id to get numeric suffix, got %q", block)
	}
}

func TestDraftFlashcardFromSnippetAppendsDraftAndSelectsDeck(t *testing.T) {
	tmp := tmpHome(t)
	m := newTestModel()
	m.config.Home = tmp
	m.config.FlashcardsEnabled = true

	source := Snippet{
		Name:     "boot-walk",
		Folder:   defaultSnippetFolder,
		File:     "boot-walk.go",
		Language: "go",
		Date:     time.Now(),
	}
	if err := os.MkdirAll(filepath.Join(tmp, defaultSnippetFolder), 0o755); err != nil {
		t.Fatalf("could not create snippet folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, defaultSnippetFolder, source.File), []byte("fmt.Println(\"boot\")\n"), 0o644); err != nil {
		t.Fatalf("could not write source snippet: %v", err)
	}
	m.Lists[Folder(defaultSnippetFolder)] = newList([]list.Item{source}, 20, m.ListStyle)
	m.Folders.SetItems([]list.Item{source})
	m.Folders.Select(0)
	m.selectSnippetInFolder(Folder(defaultSnippetFolder), source)
	m.updateKeyMap()

	got := runModelCmd(m, m.draftFlashcardFromSnippet())
	selected := got.selectedSnippet()
	if selected.File != nativeFlashcardDeckStem+".md" {
		t.Fatalf("expected deck to be selected after drafting, got %#v", selected)
	}

	deckPath := filepath.Join(tmp, defaultSnippetFolder, nativeFlashcardDeckStem+".md")
	deck, err := readNativeFlashcardDeck(deckPath)
	if err != nil {
		t.Fatalf("expected appended deck to stay valid, got %v", err)
	}
	if len(deck.Cards) != 7 {
		t.Fatalf("expected default template plus one draft card, got %d", len(deck.Cards))
	}
	draft := deck.Cards[len(deck.Cards)-1]
	if draft.ID != "draft-boot-walk" {
		t.Fatalf("unexpected appended draft id: got %q", draft.ID)
	}
	if !strings.Contains(draft.Answer, "fmt.Println(\"boot\")") {
		t.Fatalf("expected appended draft to include snippet content, got %q", draft.Answer)
	}
}

func TestCreateThenDraftFlashcardFromSameSnippet(t *testing.T) {
	tmp := tmpHome(t)
	m := newTestModel()
	m.config.Home = tmp
	m.config.FlashcardsEnabled = true

	source := Snippet{
		Name:     "boot-walk",
		Folder:   defaultSnippetFolder,
		File:     "boot-walk.go",
		Language: "go",
		Date:     time.Now(),
	}
	if err := os.MkdirAll(filepath.Join(tmp, defaultSnippetFolder), 0o755); err != nil {
		t.Fatalf("could not create snippet folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, defaultSnippetFolder, source.File), []byte("fmt.Println(\"boot\")\n"), 0o644); err != nil {
		t.Fatalf("could not write source snippet: %v", err)
	}
	m.Lists[Folder(defaultSnippetFolder)] = newList([]list.Item{source}, 20, m.ListStyle)
	m.Folders.SetItems([]list.Item{source})
	m.Folders.Select(0)
	m.selectSnippetInFolder(Folder(defaultSnippetFolder), source)
	m.updateKeyMap()

	got := runModelCmd(m, m.createFlashcardDeck())
	got = runModelCmd(got, got.draftFlashcardFromSnippet())

	selected := got.selectedSnippet()
	if selected.File != nativeFlashcardDeckStem+".md" {
		t.Fatalf("expected deck to be selected after drafting, got %#v", selected)
	}
	deckPath := filepath.Join(tmp, defaultSnippetFolder, nativeFlashcardDeckStem+".md")
	deck, err := readNativeFlashcardDeck(deckPath)
	if err != nil {
		t.Fatalf("expected created-and-drafted deck to stay valid, got %v", err)
	}
	if got := len(deck.Cards); got != 7 {
		t.Fatalf("expected default template plus one drafted card, got %d", got)
	}
	if deck.Cards[len(deck.Cards)-1].ID != "draft-boot-walk" {
		t.Fatalf("unexpected drafted card id after create+draft: got %q", deck.Cards[len(deck.Cards)-1].ID)
	}
}

func TestVisibleLineOffsetForTextIgnoresANSI(t *testing.T) {
	rendered := "top\n\x1b[38;2;1;2;3mDrafted from snippet notes/boot-walk.go. Narrow this into a smaller recall target before reviewing.\x1b[0m\nbottom"
	offset, ok := visibleLineOffsetForText(rendered, "notes/boot-walk.go")
	if !ok {
		t.Fatal("expected to find visible line containing target text")
	}
	if offset != 1 {
		t.Fatalf("unexpected offset: got %d want 1", offset)
	}
}

func TestParseNativeFlashcardDeckV2(t *testing.T) {
	deck, err := parseNativeFlashcardDeck([]byte(defaultNativeFlashcardDeckContent()))
	if err != nil {
		t.Fatalf("expected v2 deck to parse, got %v", err)
	}
	if len(deck.Cards) != 6 {
		t.Fatalf("expected six cards, got %d", len(deck.Cards))
	}
	if deck.Cards[0].Type != nativeFlashcardTypeBasic {
		t.Fatalf("expected first card to stay basic, got %q", deck.Cards[0].Type)
	}
	if deck.Cards[1].Type != nativeFlashcardTypeCodeCloze {
		t.Fatalf("expected second card to be code-cloze, got %q", deck.Cards[1].Type)
	}
	if len(deck.Cards[1].Options) != 4 {
		t.Fatalf("expected code-cloze options, got %#v", deck.Cards[1].Options)
	}
	if deck.Cards[2].Type != nativeFlashcardTypeSingleChoice {
		t.Fatalf("expected third card to be single-choice, got %q", deck.Cards[2].Type)
	}
	if len(deck.Cards[2].CorrectOptions) != 1 {
		t.Fatalf("expected single-choice card to have one correct option, got %#v", deck.Cards[2].CorrectOptions)
	}
	if deck.Cards[3].Type != nativeFlashcardTypeMultiChoice {
		t.Fatalf("expected fourth card to be multi-choice, got %q", deck.Cards[3].Type)
	}
	if len(deck.Cards[3].CorrectOptions) != 2 {
		t.Fatalf("expected multi-choice card to keep all correct options, got %#v", deck.Cards[3].CorrectOptions)
	}
	if deck.Cards[4].Type != nativeFlashcardTypeOrderedRecall {
		t.Fatalf("expected fifth card to be ordered-recall, got %q", deck.Cards[4].Type)
	}
	if len(deck.Cards[4].CorrectOptions) != len(deck.Cards[4].Options) {
		t.Fatalf("expected ordered-recall options to define the correct order, got %#v", deck.Cards[4].CorrectOptions)
	}
	if deck.Cards[5].Type != nativeFlashcardTypeTrace {
		t.Fatalf("expected sixth card to be trace, got %q", deck.Cards[5].Type)
	}
	if deck.Cards[5].Trace == "" {
		t.Fatal("expected trace card to keep trace content")
	}
}

func TestParseOrderedRecallRejectsAnswerSection(t *testing.T) {
	content := `<!-- nap-deck: v2 -->

<!-- id: boot-order -->
<!-- type: ordered-recall -->

Prompt:
Order the steps.

Options:
- firmware
- bootloader

Answer:
firmware
`

	if _, err := parseNativeFlashcardDeck([]byte(content)); err == nil {
		t.Fatal("expected ordered-recall card with answer section to be rejected")
	}
}

func TestParseTraceRejectsMissingTraceSection(t *testing.T) {
	content := `<!-- nap-deck: v2 -->

<!-- id: syscall-path -->
<!-- type: trace -->

Prompt:
What happens next?

Options:
- Enter the kernel
- Return to user space

Correct:
- Enter the kernel
`

	if _, err := parseNativeFlashcardDeck([]byte(content)); err == nil {
		t.Fatal("expected trace card without trace section to be rejected")
	} else if !strings.Contains(err.Error(), "card 1 (id syscall-path): trace cards need a Trace section") {
		t.Fatalf("expected contextual parse error, got %v", err)
	}
}

func TestParseNativeFlashcardDeckV2ErrorIncludesCardContext(t *testing.T) {
	content := `<!-- nap-deck: v2 -->

<!-- id: good-card -->
<!-- type: basic -->

Prompt:
What does CR3 point to?

Answer:
The top-level page table.

+++

<!-- id: broken-card -->
<!-- type: trace -->

Prompt:
What happens next?

Options:
- Enter the kernel
- Return to user mode

Correct:
- Enter the kernel
`

	if _, err := parseNativeFlashcardDeck([]byte(content)); err == nil {
		t.Fatal("expected invalid second card to be rejected")
	} else if !strings.Contains(err.Error(), "card 2 (id broken-card): trace cards need a Trace section") {
		t.Fatalf("expected second-card context in parse error, got %v", err)
	}
}

func TestParseNativeFlashcardDeckRejectsDuplicateCardIDs(t *testing.T) {
	content := `<!-- nap-deck: v2 -->

<!-- id: duplicate-card -->
<!-- type: basic -->

Prompt:
First prompt.

Answer:
First answer.

+++

<!-- id: duplicate-card -->
<!-- type: basic -->

Prompt:
Second prompt.

Answer:
Second answer.
`

	if _, err := parseNativeFlashcardDeck([]byte(content)); err == nil {
		t.Fatal("expected duplicate card ids to be rejected")
	} else if !strings.Contains(err.Error(), `card 2 (id duplicate-card): duplicate card id "duplicate-card"`) {
		t.Fatalf("expected duplicate id parse error, got %v", err)
	}
}

func TestSubmitOrderedRecallFlashcardShowsResult(t *testing.T) {
	m := newTestModel()
	m.flashcardSession = orderedRecallTestSession(t, orderedRecallDeckContent())

	card := m.flashcardSession.currentCard()
	for want := range card.Options {
		m.flashcardSession.Cursor = orderedRecallCursorForOption(t, m.flashcardSession, card, want)
		if cmd := m.submitNativeFlashcardAnswer(); cmd == nil {
			t.Fatal("expected ordered recall step selection to trigger a render")
		}
	}
	if m.flashcardSession.Phase != nativeFlashcardPhaseQuestion {
		t.Fatalf("expected ordered recall to stay in question phase until submission, got %v", m.flashcardSession.Phase)
	}
	if cmd := m.submitNativeFlashcardAnswer(); cmd == nil {
		t.Fatal("expected final ordered recall submission command")
	}
	if m.flashcardSession.Phase != nativeFlashcardPhaseResult {
		t.Fatalf("expected result phase, got %v", m.flashcardSession.Phase)
	}
	if got := m.nativeFlashcardResultLabel(card); got != "correct" {
		t.Fatalf("expected ordered recall result to be correct, got %q", got)
	}
}

func TestOrderedRecallIncorrectSequenceShowsIncorrect(t *testing.T) {
	m := newTestModel()
	m.flashcardSession = orderedRecallTestSession(t, orderedRecallDeckContent())

	card := m.flashcardSession.currentCard()
	for want := len(card.Options) - 1; want >= 0; want-- {
		m.flashcardSession.Cursor = orderedRecallCursorForOption(t, m.flashcardSession, card, want)
		if cmd := m.submitNativeFlashcardAnswer(); cmd == nil {
			t.Fatal("expected ordered recall step selection to trigger a render")
		}
	}
	if cmd := m.submitNativeFlashcardAnswer(); cmd == nil {
		t.Fatal("expected final ordered recall submission command")
	}
	if got := m.nativeFlashcardResultLabel(card); got != "incorrect" {
		t.Fatalf("expected ordered recall result to be incorrect, got %q", got)
	}
}

func TestOrderedRecallResultUsesCompactComparison(t *testing.T) {
	m := newTestModel()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m.state = reviewingFlashcardsState
	m.flashcardSession = orderedRecallTestSession(t, orderedRecallDeckContent())

	card := m.flashcardSession.currentCard()
	for want := len(card.Options) - 1; want >= 0; want-- {
		m.flashcardSession.Cursor = orderedRecallCursorForOption(t, m.flashcardSession, card, want)
		if cmd := m.submitNativeFlashcardAnswer(); cmd == nil {
			t.Fatal("expected ordered recall step selection to trigger a render")
		} else {
			m = runModelCmd(m, cmd)
		}
	}
	if cmd := m.submitNativeFlashcardAnswer(); cmd == nil {
		t.Fatal("expected final ordered recall submission command")
	} else {
		m = runModelCmd(m, cmd)
	}

	view := m.Code.View()
	if !strings.Contains(view, "order check") {
		t.Fatalf("expected compact ordered recall comparison, got %q", view)
	}
	if !strings.Contains(view, "you put it at 3") {
		t.Fatalf("expected ordered recall mismatch note, got %q", view)
	}
	if strings.Contains(view, "build order") || strings.Contains(view, "your order") || strings.Contains(view, "correct order") {
		t.Fatalf("expected verbose ordered recall sections to be removed from result view, got %q", view)
	}
}

func TestGradeOrderedRecallResetsSequenceForNextCard(t *testing.T) {
	tmp := tmpHome(t)
	if err := os.MkdirAll(filepath.Join(tmp, defaultSnippetFolder), 0o755); err != nil {
		t.Fatalf("could not create snippet folder: %v", err)
	}

	m := newTestModel()
	m.config.Home = tmp
	m.flashcardSession = orderedRecallTestSession(t, orderedRecallDeckContent()+`

+++

<!-- id: next-basic -->
<!-- type: basic -->

Prompt:
What does CR3 point to?

Answer:
The base of the top-level page table.
`)

	card := m.flashcardSession.currentCard()
	for want := range card.Options {
		m.flashcardSession.Cursor = orderedRecallCursorForOption(t, m.flashcardSession, card, want)
		if cmd := m.submitNativeFlashcardAnswer(); cmd == nil {
			t.Fatal("expected ordered recall step selection to trigger a render")
		}
	}
	if cmd := m.submitNativeFlashcardAnswer(); cmd == nil {
		t.Fatal("expected ordered recall submission command")
	}
	if cmd := m.gradeNativeFlashcard(flashcardGradeGood); cmd == nil {
		t.Fatal("expected grading command")
	}
	if got := m.flashcardSession.currentCard().ID; got != "next-basic" {
		t.Fatalf("expected grading to advance to the next card, got %q", got)
	}
	if len(m.flashcardSession.OrderedSequence) != 0 {
		t.Fatalf("expected ordered recall sequence to reset, got %#v", m.flashcardSession.OrderedSequence)
	}
	if len(m.flashcardSession.Selections) != 0 {
		t.Fatalf("expected selections to reset, got %#v", m.flashcardSession.Selections)
	}
}

func TestRemoveNativeFlashcardOrderedStepNoopWhenEmpty(t *testing.T) {
	m := newTestModel()
	m.flashcardSession = orderedRecallTestSession(t, orderedRecallDeckContent())

	if cmd := m.removeNativeFlashcardOrderedStep(); cmd != nil {
		t.Fatalf("expected removing from an empty ordered sequence to be a no-op, got %T", cmd())
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

func TestReviewFlashcardsShowsContextualDeckValidationError(t *testing.T) {
	tmp := tmpHome(t)
	m := newTestModel()
	m.config.Home = tmp
	m.config.FlashcardsEnabled = true
	m.Update(tea.WindowSizeMsg{Width: 220, Height: 40})
	folder := Folder(defaultSnippetFolder)
	deck := Snippet{Name: nativeFlashcardDeckStem, Folder: defaultSnippetFolder, File: nativeFlashcardDeckStem + ".md", Language: "md", Date: time.Now()}
	if err := os.MkdirAll(filepath.Join(tmp, defaultSnippetFolder), 0o755); err != nil {
		t.Fatalf("could not create deck folder: %v", err)
	}
	content := `<!-- nap-deck: v2 -->

<!-- id: duplicate-card -->
<!-- type: basic -->

Prompt:
First prompt.

Answer:
First answer.

+++

<!-- id: duplicate-card -->
<!-- type: basic -->

Prompt:
Second prompt.

Answer:
Second answer.
`
	if err := os.WriteFile(filepath.Join(tmp, defaultSnippetFolder, deck.File), []byte(content), 0o644); err != nil {
		t.Fatalf("could not write invalid deck: %v", err)
	}
	m.Lists[folder] = newList([]list.Item{deck}, 20, m.ListStyle)
	m.Folders.SetItems([]list.Item{folder})
	m.Folders.Select(0)

	if cmd := m.reviewFlashcards(); cmd != nil {
		t.Fatalf("expected invalid deck review to stop with an error, got %T", cmd())
	}
	if view := m.Code.View(); !strings.Contains(view, `card 2 (id duplicate-card): duplicate card id "duplicate-card"`) {
		t.Fatalf("expected contextual validation error in UI, got %q", view)
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

func TestDeleteSelectedSnippetRemovesNativeState(t *testing.T) {
	tmp := tmpHome(t)
	m := newTestModel()
	m.config.Home = tmp
	m.config.FlashcardsEnabled = true
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	folder := Folder(defaultSnippetFolder)
	deck := Snippet{Name: nativeFlashcardDeckStem, Folder: defaultSnippetFolder, File: nativeFlashcardDeckStem + ".md", Language: "md", Date: time.Now()}
	if err := os.MkdirAll(filepath.Join(tmp, defaultSnippetFolder), 0o755); err != nil {
		t.Fatalf("could not create deck folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, defaultSnippetFolder, deck.File), []byte(defaultNativeFlashcardDeckContent()), 0o644); err != nil {
		t.Fatalf("could not write native deck: %v", err)
	}
	if err := writeNativeFlashcardState(tmp, deck, nativeFlashcardState{
		Cards: map[string]nativeFlashcardProgress{
			"linux-paging-4level-walk": {Reviews: 1, LastGrade: flashcardGradeGood, DueAt: time.Now().Add(24 * time.Hour)},
		},
	}); err != nil {
		t.Fatalf("could not write native state: %v", err)
	}
	m.Lists[folder] = newList([]list.Item{deck}, 20, m.ListStyle)
	m.Folders.SetItems([]list.Item{deck})
	m.Folders.Select(0)
	m.Lists[folder].Select(0)

	if cmd := m.deleteSelectedSnippet(); cmd == nil {
		t.Fatal("expected delete selected snippet command")
	} else {
		_ = cmd()
	}

	statePath, err := nativeFlashcardStatePath(tmp, deck)
	if err != nil {
		t.Fatalf("could not resolve native state path: %v", err)
	}
	if _, err := os.Stat(statePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected native state file to be removed with deck deletion, got err=%v", err)
	}
}

func TestNativeFlashcardIndicatorStatesUsesRecallForHardGrade(t *testing.T) {
	tmp := tmpHome(t)
	folder := defaultSnippetFolder
	deck := Snippet{Name: nativeFlashcardDeckStem, Folder: folder, File: nativeFlashcardDeckStem + ".md", Language: "md", Date: time.Now()}
	if err := os.MkdirAll(filepath.Join(tmp, folder), 0o755); err != nil {
		t.Fatalf("could not create deck folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, folder, deck.File), []byte(defaultNativeFlashcardDeckContent()), 0o644); err != nil {
		t.Fatalf("could not write native deck: %v", err)
	}
	if err := writeNativeFlashcardState(tmp, deck, nativeFlashcardState{
		Cards: map[string]nativeFlashcardProgress{
			"linux-paging-4level-walk": {Reviews: 1, LastGrade: flashcardGradeHard, DueAt: time.Now().Add(24 * time.Hour)},
		},
	}); err != nil {
		t.Fatalf("could not write native state: %v", err)
	}

	states := nativeFlashcardIndicatorStates(tmp, deck, time.Now())
	if len(states) != 1 || states[0] != flashcardDeckRecall {
		t.Fatalf("expected hard grade to produce recall state, got %#v", states)
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
	m.flashcardSession.Phase = nativeFlashcardPhaseResult

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

func TestSubmitSingleChoiceFlashcardAnswerShowsResult(t *testing.T) {
	tmp := tmpHome(t)
	m := newTestModel()
	m.config.Home = tmp
	m.config.FlashcardsEnabled = true
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	deck := Snippet{Name: nativeFlashcardDeckStem, Folder: defaultSnippetFolder, File: nativeFlashcardDeckStem + ".md", Language: "md", Date: time.Now()}
	if err := os.MkdirAll(filepath.Join(tmp, defaultSnippetFolder), 0o755); err != nil {
		t.Fatalf("could not create deck folder: %v", err)
	}
	content := `<!-- nap-deck: v2 -->

<!-- id: mmap-private-anon -->
<!-- type: code-cloze -->

Prompt:
Which flags create an anonymous private mapping?

Options:
- MAP_SHARED
- MAP_FIXED
- MAP_PRIVATE | MAP_ANONYMOUS
- MAP_HUGETLB

Answer:
MAP_PRIVATE | MAP_ANONYMOUS
`
	if err := os.WriteFile(filepath.Join(tmp, defaultSnippetFolder, deck.File), []byte(content), 0o644); err != nil {
		t.Fatalf("could not write deck: %v", err)
	}
	m.Lists[Folder(defaultSnippetFolder)] = newList([]list.Item{deck}, 20, m.ListStyle)
	m.Folders.SetItems([]list.Item{Folder(defaultSnippetFolder)})
	m.Folders.Select(0)

	if cmd := m.reviewFlashcards(); cmd == nil {
		t.Fatal("expected native review command")
	}
	m.flashcardSession.Cursor = 2
	if cmd := m.submitNativeFlashcardAnswer(); cmd == nil {
		t.Fatal("expected submit answer command")
	} else {
		m = runModelCmd(m, cmd)
	}
	if m.flashcardSession.Phase != nativeFlashcardPhaseResult {
		t.Fatalf("expected result phase, got %v", m.flashcardSession.Phase)
	}
	if !m.flashcardSession.selected(2) {
		t.Fatal("expected selected answer to be recorded")
	}
	if view := m.Code.View(); !strings.Contains(view, m.ContentStyle.FlashcardPositive.Render("correct")) {
		t.Fatalf("expected correct result label to be highlighted, got %q", view)
	}
}

func TestNativeFlashcardIncorrectResultUsesNegativeStyle(t *testing.T) {
	tmp := tmpHome(t)
	m := newTestModel()
	m.config.Home = tmp
	m.config.FlashcardsEnabled = true
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	deck := Snippet{Name: nativeFlashcardDeckStem, Folder: defaultSnippetFolder, File: nativeFlashcardDeckStem + ".md", Language: "md", Date: time.Now()}
	if err := os.MkdirAll(filepath.Join(tmp, defaultSnippetFolder), 0o755); err != nil {
		t.Fatalf("could not create deck folder: %v", err)
	}
	content := `<!-- nap-deck: v2 -->

<!-- id: mmap-private-anon -->
<!-- type: code-cloze -->

Prompt:
Which flags create an anonymous private mapping?

Options:
- MAP_SHARED
- MAP_FIXED
- MAP_PRIVATE | MAP_ANONYMOUS
- MAP_HUGETLB

Answer:
MAP_PRIVATE | MAP_ANONYMOUS
`
	if err := os.WriteFile(filepath.Join(tmp, defaultSnippetFolder, deck.File), []byte(content), 0o644); err != nil {
		t.Fatalf("could not write deck: %v", err)
	}
	m.Lists[Folder(defaultSnippetFolder)] = newList([]list.Item{deck}, 20, m.ListStyle)
	m.Folders.SetItems([]list.Item{Folder(defaultSnippetFolder)})
	m.Folders.Select(0)

	if cmd := m.reviewFlashcards(); cmd == nil {
		t.Fatal("expected native review command")
	}
	m.flashcardSession.Cursor = 0
	if cmd := m.submitNativeFlashcardAnswer(); cmd == nil {
		t.Fatal("expected submit answer command")
	} else {
		m = runModelCmd(m, cmd)
	}
	if view := m.Code.View(); !strings.Contains(view, m.ContentStyle.FlashcardNegative.Render("incorrect")) {
		t.Fatalf("expected incorrect result label to be highlighted, got %q", view)
	}
}

func TestSubmitTraceChoiceFlashcardShowsTraceAndCorrectResult(t *testing.T) {
	tmp := tmpHome(t)
	m := newTestModel()
	m.config.Home = tmp
	m.config.FlashcardsEnabled = true
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	deck := Snippet{Name: nativeFlashcardDeckStem, Folder: defaultSnippetFolder, File: nativeFlashcardDeckStem + ".md", Language: "md", Date: time.Now()}
	if err := os.MkdirAll(filepath.Join(tmp, defaultSnippetFolder), 0o755); err != nil {
		t.Fatalf("could not create deck folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, defaultSnippetFolder, deck.File), []byte(traceChoiceDeckContent()), 0o644); err != nil {
		t.Fatalf("could not write deck: %v", err)
	}
	m.Lists[Folder(defaultSnippetFolder)] = newList([]list.Item{deck}, 20, m.ListStyle)
	m.Folders.SetItems([]list.Item{Folder(defaultSnippetFolder)})
	m.Folders.Select(0)

	if cmd := m.reviewFlashcards(); cmd == nil {
		t.Fatal("expected native review command")
	} else {
		m = runModelCmd(m, cmd)
	}
	if view := m.Code.View(); !strings.Contains(view, "trace") || !strings.Contains(view, "glibc wrapper") {
		t.Fatalf("expected trace content in question view, got %q", view)
	}
	m.flashcardSession.Cursor = 1
	if cmd := m.submitNativeFlashcardAnswer(); cmd == nil {
		t.Fatal("expected submit answer command")
	} else {
		m = runModelCmd(m, cmd)
	}
	if m.flashcardSession.Phase != nativeFlashcardPhaseResult {
		t.Fatalf("expected result phase, got %v", m.flashcardSession.Phase)
	}
	if view := m.Code.View(); !strings.Contains(view, m.ContentStyle.FlashcardPositive.Render("correct")) {
		t.Fatalf("expected correct trace result label, got %q", view)
	}
	if view := m.Code.View(); !strings.Contains(view, "glibc wrapper") {
		t.Fatalf("expected trace content to stay visible in result view, got %q", view)
	}
}

func TestSubmitTraceRevealFlashcardShowsRevealedResult(t *testing.T) {
	m := newTestModel()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m.state = reviewingFlashcardsState
	m.flashcardSession = flashcardTestSession(t, traceRevealDeckContent())

	card := m.flashcardSession.currentCard()
	if cmd := m.submitNativeFlashcardAnswer(); cmd == nil {
		t.Fatal("expected reveal trace command")
	} else {
		m = runModelCmd(m, cmd)
	}
	if got := m.nativeFlashcardResultLabel(card); got != "revealed" {
		t.Fatalf("expected reveal-only trace to be marked revealed, got %q", got)
	}
	if view := m.Code.View(); !strings.Contains(view, m.ContentStyle.FlashcardPending.Render("revealed")) {
		t.Fatalf("expected revealed trace result label, got %q", view)
	}
	if view := m.Code.View(); !strings.Contains(view, "page-fault exception") {
		t.Fatalf("expected trace answer in result view, got %q", view)
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

func flashcardTestSession(t *testing.T, content string) *nativeFlashcardReviewSession {
	t.Helper()

	parsed, err := parseNativeFlashcardDeck([]byte(content))
	if err != nil {
		t.Fatalf("could not parse flashcard deck: %v", err)
	}
	session, err := buildNativeFlashcardReviewSession(
		Snippet{
			Name:     nativeFlashcardDeckStem,
			Folder:   defaultSnippetFolder,
			File:     nativeFlashcardDeckStem + ".md",
			Language: "md",
			Date:     time.Now(),
		},
		parsed,
		nativeFlashcardState{Cards: map[string]nativeFlashcardProgress{}},
		time.Now(),
	)
	if err != nil {
		t.Fatalf("could not build flashcard session: %v", err)
	}
	return session
}

func orderedRecallTestSession(t *testing.T, content string) *nativeFlashcardReviewSession {
	t.Helper()
	return flashcardTestSession(t, content)
}

func orderedRecallCursorForOption(t *testing.T, session *nativeFlashcardReviewSession, card nativeFlashcard, want int) int {
	t.Helper()

	for position, idx := range session.displayOrder(card) {
		if idx == want {
			return position
		}
	}
	t.Fatalf("could not find option %d in display order %#v", want, session.displayOrder(card))
	return 0
}

func orderedRecallDeckContent() string {
	return `<!-- nap-deck: v2 -->

<!-- id: boot-order -->
<!-- type: ordered-recall -->

Prompt:
Order the boot steps.

Options:
- firmware
- bootloader
- kernel
`
}

func traceChoiceDeckContent() string {
	return `<!-- nap-deck: v2 -->

<!-- id: syscall-openat-trace -->
<!-- type: trace -->

Prompt:
What happens next?

Trace:
userspace -> glibc wrapper -> syscall instruction

Options:
- Return directly to userspace.
- Enter the syscall dispatch path.
- Jump into the block layer.

Correct:
- Enter the syscall dispatch path.
`
}

func traceRevealDeckContent() string {
	return `<!-- nap-deck: v2 -->

<!-- id: page-fault-trace -->
<!-- type: trace -->

Prompt:
What fault path is this describing?

Trace:
CPU loads from a not-present user page
hardware raises #PF

Answer:
The CPU raises a page-fault exception and control transfers into the kernel page-fault handler.
`
}
