package nap

import "github.com/charmbracelet/bubbles/key"

// KeyMap is the mappings of actions to key bindings.
type KeyMap struct {
	Quit          key.Binding
	Search        key.Binding
	ToggleHelp    key.Binding
	NewSnippet    key.Binding
	NewFolder     key.Binding
	NewRootFolder key.Binding
	DeleteSnippet key.Binding
	DeleteFolder  key.Binding
	EditSnippet   key.Binding
	CopySnippet   key.Binding
	PasteSnippet  key.Binding
	SetFolder     key.Binding
	RenameSnippet key.Binding
	TagSnippet    key.Binding
	SetLanguage   key.Binding
	Confirm       key.Binding
	Cancel        key.Binding
	NextPane      key.Binding
	PreviousPane  key.Binding
	ChangeFolder  key.Binding
}

// DefaultKeyMap is the default key map for the application.
var DefaultKeyMap = KeyMap{
	Quit:          key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "exit")),
	Search:        key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
	ToggleHelp:    key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	NewSnippet:    key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new snippet")),
	NewFolder:     key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "new folder")),
	NewRootFolder: key.NewBinding(key.WithKeys("O"), key.WithHelp("O", "new root folder")),
	DeleteSnippet: key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "del snippet")),
	DeleteFolder:  key.NewBinding(key.WithKeys("X"), key.WithHelp("X", "del folder")),
	EditSnippet:   key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit")),
	CopySnippet:   key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "copy")),
	PasteSnippet:  key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "paste")),
	RenameSnippet: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rename snippet")),
	SetFolder:     key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "rename folder")),
	SetLanguage:   key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "set file type")),
	TagSnippet:    key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "tag"), key.WithDisabled()),
	Confirm:       key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "confirm")),
	Cancel:        key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	NextPane:      key.NewBinding(key.WithKeys("l", "tab", "right"), key.WithHelp("l", "left")),
	PreviousPane:  key.NewBinding(key.WithKeys("h", "shift+tab", "left"), key.WithHelp("h", "right")),
	ChangeFolder:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "change folder"), key.WithDisabled()),
}

// ShortHelp returns a quick help menu.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
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
}

func shortHelpBinding(binding key.Binding) key.Binding {
	return key.NewBinding(
		key.WithKeys(binding.Keys()...),
		key.WithHelp(binding.Help().Key, binding.Help().Desc),
	)
}

// FullHelp returns all help options in a more detailed view.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.NewSnippet, k.NewFolder, k.NewRootFolder, k.EditSnippet, k.PasteSnippet},
		{k.DeleteSnippet, k.DeleteFolder},
		{k.RenameSnippet, k.SetFolder, k.TagSnippet, k.SetLanguage},
		{k.NextPane, k.PreviousPane},
		{k.Search, k.ToggleHelp, k.Quit},
	}
}
