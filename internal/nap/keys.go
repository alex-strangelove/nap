package nap

import "github.com/charmbracelet/bubbles/key"

// KeyMap is the mappings of actions to key bindings.
type KeyMap struct {
	Quit              key.Binding
	SearchPreview     key.Binding
	SearchMetadata    key.Binding
	SearchContents    key.Binding
	ToggleHelp        key.Binding
	NewSnippet        key.Binding
	CreateFlashcards  key.Binding
	ReviewFlashcards  key.Binding
	ResetFlashcards   key.Binding
	NewFolder         key.Binding
	NewRootFolder     key.Binding
	DeleteSnippet     key.Binding
	DeleteFolder      key.Binding
	EditSnippet       key.Binding
	CopySnippet       key.Binding
	PasteSnippet      key.Binding
	SetFolder         key.Binding
	RenameSnippet     key.Binding
	TagSnippet        key.Binding
	SetLanguage       key.Binding
	Confirm           key.Binding
	Cancel            key.Binding
	NextPane          key.Binding
	PreviousPane      key.Binding
	ChangeFolder      key.Binding
	SearchNext        key.Binding
	SearchPrevious    key.Binding
	SearchEdit        key.Binding
	SearchFocusLeft   key.Binding
	SearchFocusRight  key.Binding
	flashcardsEnabled bool
}

// DefaultKeyMap is the default key map for the application.
var DefaultKeyMap = KeyMap{
	Quit:             key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "exit")),
	SearchPreview:    key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "find in file")),
	SearchMetadata:   key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "search files")),
	SearchContents:   key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "search contents")),
	ToggleHelp:       key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	NewSnippet:       key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new snippet")),
	CreateFlashcards: key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "new cards"), key.WithDisabled()),
	ReviewFlashcards: key.NewBinding(key.WithKeys("F"), key.WithHelp("F", "review cards"), key.WithDisabled()),
	ResetFlashcards:  key.NewBinding(key.WithKeys("z"), key.WithHelp("z", "reset cards"), key.WithDisabled()),
	NewFolder:        key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "new folder")),
	NewRootFolder:    key.NewBinding(key.WithKeys("O"), key.WithHelp("O", "new root folder")),
	DeleteSnippet:    key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "del snippet")),
	DeleteFolder:     key.NewBinding(key.WithKeys("X"), key.WithHelp("X", "del folder")),
	EditSnippet:      key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit")),
	CopySnippet:      key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "copy")),
	PasteSnippet:     key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "paste")),
	RenameSnippet:    key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rename snippet")),
	SetFolder:        key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "rename folder")),
	SetLanguage:      key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "set file type")),
	TagSnippet:       key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "tag"), key.WithDisabled()),
	Confirm:          key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "confirm")),
	Cancel:           key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	NextPane:         key.NewBinding(key.WithKeys("l", "tab", "right"), key.WithHelp("l", "left")),
	PreviousPane:     key.NewBinding(key.WithKeys("h", "shift+tab", "left"), key.WithHelp("h", "right")),
	ChangeFolder:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "change folder"), key.WithDisabled()),
	SearchNext:       key.NewBinding(key.WithKeys("ctrl+j"), key.WithHelp("ctrl+j", "next hit"), key.WithDisabled()),
	SearchPrevious:   key.NewBinding(key.WithKeys("ctrl+k"), key.WithHelp("ctrl+k", "prev hit"), key.WithDisabled()),
	SearchEdit:       key.NewBinding(key.WithKeys("ctrl+e"), key.WithHelp("ctrl+e", "edit match"), key.WithDisabled()),
	SearchFocusLeft:  key.NewBinding(key.WithKeys("ctrl+h"), key.WithHelp("ctrl+h", "focus search"), key.WithDisabled()),
	SearchFocusRight: key.NewBinding(key.WithKeys("ctrl+l"), key.WithHelp("ctrl+l", "focus preview"), key.WithDisabled()),
}

// ShortHelp returns a quick help menu.
func (k KeyMap) ShortHelp() []key.Binding {
	shortHelp := []key.Binding{
		shortHelpBinding(k.NextPane),
		shortHelpBinding(k.PreviousPane),
		shortHelpBinding(k.EditSnippet),
		shortHelpBinding(k.NewSnippet),
		shortHelpBinding(k.DeleteSnippet),
		shortHelpBinding(k.NewFolder),
		shortHelpBinding(k.NewRootFolder),
		shortHelpBinding(k.DeleteFolder),
		shortHelpBinding(k.CopySnippet),
		shortHelpBinding(k.ToggleHelp),
	}

	if k.flashcardsEnabled {
		for _, binding := range []key.Binding{k.CreateFlashcards, k.ReviewFlashcards, k.ResetFlashcards} {
			if binding.Enabled() {
				shortHelp = append(shortHelp, shortHelpBinding(binding))
			}
		}
	}

	return shortHelp
}

func shortHelpBinding(binding key.Binding) key.Binding {
	return key.NewBinding(
		key.WithKeys(binding.Keys()...),
		key.WithHelp(binding.Help().Key, binding.Help().Desc),
	)
}

// FullHelp returns all help options in a more detailed view.
func (k KeyMap) FullHelp() [][]key.Binding {
	firstRow := []key.Binding{k.NewSnippet, k.NewFolder, k.NewRootFolder, k.EditSnippet, k.PasteSnippet}
	if k.flashcardsEnabled {
		firstRow = append(firstRow, k.CreateFlashcards, k.ReviewFlashcards, k.ResetFlashcards)
	}

	return [][]key.Binding{
		firstRow,
		{k.DeleteSnippet, k.DeleteFolder},
		{k.RenameSnippet, k.SetFolder, k.TagSnippet, k.SetLanguage},
		{k.NextPane, k.PreviousPane},
		{k.SearchPreview, k.SearchMetadata, k.SearchContents, k.SearchEdit, k.SearchFocusLeft, k.SearchFocusRight, k.ToggleHelp, k.Quit},
	}
}
