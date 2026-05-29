package nap

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mattn/go-isatty"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
)

var (
	helpText = strings.TrimSpace(`
Nap is a code snippet manager for your terminal.
https://github.com/maaslalani/nap

Usage:
  nap           - for interactive mode
  nap list      - list all snippets
  nap <snippet> - print snippet to stdout

Create:
  nap < main.go                 - save snippet from stdin
  nap example/main.go < main.go - save snippet with name`)
)

func Main() {
	runCLI(os.Args[1:])
}

func runCLI(args []string) {
	config := readConfig()
	snippets := readSnippets(config)
	snippets = migrateSnippets(config, snippets)
	snippets = scanSnippets(config, snippets)

	stdin := readStdin()
	if stdin != "" {
		saveSnippet(stdin, args, config, snippets)
		return
	}

	if len(args) > 0 {
		switch args[0] {
		case "list":
			listSnippets(snippets)
		case "-h", "--help":
			fmt.Println(helpText)
		default:
			snippet := findSnippet(args[0], snippets)
			fmt.Print(snippet.Content(isatty.IsTerminal(os.Stdout.Fd())))
		}
		return
	}

	err := runInteractiveMode(config, snippets)
	if err != nil {
		fmt.Println("Alas, there's been an error", err)
	}
}

// parseName returns a folder, name, and language for the given name.
// this is useful for parsing file names when passed as command line arguments.
//
// Example:
//
//	Notes/Hello.go -> (Notes, Hello, go)
//	Hello.go       -> (Misc, Hello, go)
//	Notes/Hello    -> (Notes, Hello, go)
func parseName(s string) (string, string, string, error) {
	var (
		folder   = defaultSnippetFolder
		name     = defaultSnippetName
		language = defaultLanguage
	)

	clean := filepath.Clean(s)
	base := filepath.Base(clean)

	if dir := filepath.Dir(clean); dir != "." {
		folder = filepath.ToSlash(dir)
	}

	ext := filepath.Ext(base)
	if ext != "" {
		language = strings.TrimPrefix(ext, ".")
		base = strings.TrimSuffix(base, ext)
	}

	if base != "" && base != "." {
		name = base
	}

	if err := validateSnippetRelativePath(filepath.Join(folder, fmt.Sprintf("%s.%s", name, language))); err != nil {
		return "", "", "", err
	}

	return folder, name, language, nil
}

// readStdin returns the stdin that is piped in to the command line interface.
func readStdin() string {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return ""
	}

	if stat.Mode()&os.ModeCharDevice != 0 {
		return ""
	}

	reader := bufio.NewReader(os.Stdin)
	var b strings.Builder

	for {
		r, _, err := reader.ReadRune()
		if err != nil && err == io.EOF {
			break
		}
		_, err = b.WriteRune(r)
		if err != nil {
			return ""
		}
	}

	return b.String()
}

// readSnippets returns all the snippets read from the snippets.json file.
func readSnippets(config Config) []Snippet {
	var snippets []Snippet
	file := filepath.Join(config.Home, config.File)
	dir, err := os.ReadFile(file)
	if err != nil {
		// File does not exist, create one.
		err := os.MkdirAll(config.Home, os.ModePerm)
		if err != nil {
			fmt.Printf("Unable to create directory %s, %+v", config.Home, err)
		}
		f, err := os.Create(file)
		if err != nil {
			fmt.Printf("Unable to create file %s, %+v", file, err)
		}
		defer f.Close()
		dir = []byte("[]")
		_, _ = f.Write(dir)
	}
	err = json.Unmarshal(dir, &snippets)
	if err != nil {
		fmt.Printf("Unable to unmarshal %s file, %+v\n", file, err)
		return snippets
	}
	return snippets
}

// migrateSnippets migrates any legacy snippet <dir>-<file> format to the new <dir>/<file> format
func migrateSnippets(config Config, snippets []Snippet) []Snippet {
	var migrated bool
	for idx, snippet := range snippets {
		legacyPath := filepath.Join(config.Home, snippet.LegacyPath())
		if _, err := os.Stat(legacyPath); err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				fmt.Printf("could not access %q: %v\n", legacyPath, err)
			}
			continue
		}
		file := strings.TrimPrefix(snippet.LegacyPath(), fmt.Sprintf("%s-", snippet.Folder))
		newDir := filepath.Join(config.Home, snippet.Folder)
		newPath := filepath.Join(newDir, file)
		if err := os.MkdirAll(newDir, os.ModePerm); err != nil {
			fmt.Printf("could not create %q: %v\n", newDir, err)
			continue
		}
		if err := os.Rename(legacyPath, newPath); err != nil {
			fmt.Printf("could not move %q to %q: %v\n", legacyPath, newPath, err)
		}
		migrated = true
		snippet.File = file
		snippets[idx] = snippet
	}
	if migrated {
		writeSnippets(config, snippets)
	}
	return snippets
}

// scanSnippets scans for any new/removed snippets and adds them to snippets.json
func scanSnippets(config Config, snippets []Snippet) []Snippet {
	var modified bool
	snippetsPath := filepath.Clean(filepath.Join(config.Home, config.File))
	snippetExists := func(path string) bool {
		for _, snippet := range snippets {
			if path == snippet.Path() {
				return true
			}
		}
		return false
	}

	err := filepath.WalkDir(config.Home, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			fmt.Printf("could not scan %q: %v\n", path, err)
			return nil
		}

		if path == config.Home {
			return nil
		}

		if strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if entry.IsDir() {
			return nil
		}

		if filepath.Clean(path) == snippetsPath {
			return nil
		}

		relPath, err := filepath.Rel(config.Home, path)
		if err != nil {
			fmt.Printf("could not derive relative path for %q: %v\n", path, err)
			return nil
		}

		folder := filepath.Dir(relPath)
		if folder == "." {
			return nil
		}

		snippetPath := filepath.Clean(relPath)
		if snippetExists(snippetPath) {
			return nil
		}

		name := filepath.Base(relPath)
		ext := filepath.Ext(name)
		snippets = append(snippets, Snippet{
			Folder:   filepath.ToSlash(folder),
			Date:     time.Now(),
			Name:     strings.TrimSuffix(name, ext),
			File:     name,
			Language: strings.TrimPrefix(ext, "."),
			Tags:     make([]string, 0),
		})
		modified = true
		return nil
	})
	if err != nil {
		fmt.Printf("could not scan config home: %v\n", err)
		return snippets
	}

	var idx int
	for _, snippet := range snippets {
		snippetPath, err := snippetStoragePath(config.Home, snippet)
		if err != nil {
			modified = true
			continue
		}
		if _, err := os.Stat(snippetPath); !errors.Is(err, fs.ErrNotExist) {
			snippets[idx] = snippet
			idx++
		}
	}
	if idx != len(snippets) {
		modified = true
	}
	snippets = snippets[:idx]

	if modified {
		writeSnippets(config, snippets)
	}

	return snippets
}

func saveSnippet(content string, args []string, config Config, snippets []Snippet) {
	// Save snippet to location
	name := defaultSnippetName
	if len(args) > 0 {
		name = strings.Join(args, " ")
	}

	folder, name, language, err := parseName(name)
	if err != nil {
		fmt.Println(err)
		return
	}
	file := fmt.Sprintf("%s.%s", name, language)
	filePath, err := resolveHomePath(config.Home, filepath.Join(folder, file))
	if err != nil {
		fmt.Println(err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
		fmt.Println("unable to create folder")
		return
	}
	err = os.WriteFile(filePath, []byte(content), 0o644)
	if err != nil {
		fmt.Println("unable to create snippet")
		return
	}

	// Add snippet metadata
	snippet := Snippet{
		Folder:   folder,
		Date:     time.Now(),
		Name:     name,
		File:     file,
		Language: language,
	}

	snippets = append([]Snippet{snippet}, snippets...)
	writeSnippets(config, snippets)
}

func writeSnippets(config Config, snippets []Snippet) {
	snippets = append([]Snippet(nil), snippets...)
	sortSnippets(snippets)
	b, err := json.Marshal(snippets)
	if err != nil {
		fmt.Println("Could not marshal latest snippet data.", err)
		return
	}
	err = os.WriteFile(filepath.Join(config.Home, config.File), b, os.ModePerm)
	if err != nil {
		fmt.Println("Could not save snippets file.", err)
	}
}

func listSnippets(snippets []Snippet) {
	for _, snippet := range snippets {
		fmt.Println(snippet)
	}
}

func findSnippet(search string, snippets []Snippet) Snippet {
	matches := fuzzy.FindFrom(search, Snippets{snippets})
	if len(matches) > 0 {
		return snippets[matches[0].Index]
	}
	return Snippet{}
}

func initialInteractiveSelection(state State, lists map[Folder]*list.Model, tree folderTree) (Folder, list.Item) {
	currentFolder := Folder(defaultSnippetFolder)
	if state.CurrentFolder != "" {
		candidate := Folder(state.CurrentFolder)
		if _, ok := lists[candidate]; ok {
			currentFolder = candidate
		}
	}

	if _, ok := lists[currentFolder]; !ok {
		if len(tree.roots) > 0 {
			currentFolder = tree.roots[0]
		} else {
			for folder := range lists {
				currentFolder = folder
				break
			}
		}
	}

	currentItem := list.Item(currentFolder)
	if snippetList, ok := lists[currentFolder]; ok {
		for _, item := range snippetList.Items() {
			snippet, ok := item.(Snippet)
			if ok && snippet.File == state.CurrentSnippet {
				currentItem = snippet
				break
			}
		}
	}

	return currentFolder, currentItem
}

func runInteractiveMode(config Config, snippets []Snippet) error {
	if len(snippets) == 0 {
		// welcome to nap!
		snippets = append(snippets, defaultSnippet)
	}
	state := readState()

	folders := make(map[Folder][]list.Item)
	for _, snippet := range snippets {
		folders[Folder(snippet.Folder)] = append(folders[Folder(snippet.Folder)], list.Item(snippet))
	}

	defaultStyles := DefaultStyles(config)
	lists := map[Folder]*list.Model{}
	for folder, items := range folders {
		lists[folder] = newList(items, 20, defaultStyles.Snippets.Focused)
	}
	ensureAncestorLists(lists, 20, defaultStyles.Snippets.Focused)

	folderExpanded := make(map[Folder]bool, len(state.ExpandedFolders))
	for _, folder := range state.ExpandedFolders {
		folderExpanded[Folder(folder)] = true
	}

	tree := buildFolderTree(lists)
	currentFolder, currentItem := initialInteractiveSelection(state, lists, tree)
	for _, ancestor := range ancestorFolders(currentFolder) {
		folderExpanded[ancestor] = true
	}

	folderItems := tree.visibleItems(folderExpanded)
	folderList := list.New(folderItems, folderDelegate{
		styles:   defaultStyles.Folders.Blurred,
		depths:   tree.depths,
		expanded: folderExpanded,
		children: tree.children,
		snippets: tree.snippets,
	}, 0, 0)
	folderList.Title = "Folders"

	folderList.SetShowHelp(false)
	folderList.SetFilteringEnabled(false)
	folderList.SetShowStatusBar(false)
	folderList.DisableQuitKeybindings()
	folderList.Styles.NoItems = lipgloss.NewStyle().Margin(0, 2).Foreground(lipgloss.Color(config.GrayColor))
	folderList.SetStatusBarItemName("folder", "folders")
	folderList.Select(visibleFolderIndex(folderItems, currentItem, tree.parents))

	content := viewport.New(80, 0)

	currentFolder = treeItemFolder(folderList.SelectedItem())
	for folder, snippetList := range lists {
		if folder != currentFolder {
			continue
		}
		for idx, item := range snippetList.Items() {
			if s, ok := item.(Snippet); ok && s.File == state.CurrentSnippet {
				snippetList.Select(idx)
				break
			}
		}
	}

	m := &Model{
		Lists:          lists,
		Folders:        folderList,
		folderTree:     tree,
		folderExpanded: folderExpanded,
		Code:           content,
		ContentStyle:   defaultStyles.Content.Blurred,
		ListStyle:      defaultStyles.Snippets.Focused,
		FoldersStyle:   defaultStyles.Folders.Blurred,
		keys:           DefaultKeyMap,
		help:           help.New(),
		config:         config,
		inputs: []textinput.Model{
			newTextInput(defaultSnippetFolder + " "),
			newTextInput(defaultSnippetName + " "),
			newTextInput(config.DefaultLanguage),
		},
		tagsInput:    newTextInput("Tags"),
		contentCache: map[contentCacheKey]contentCacheEntry{},
		pane:         folderPane,
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	model, err := p.Run()
	if err != nil {
		return err
	}
	fm, ok := model.(*Model)
	if !ok {
		return err
	}
	var allSnippets []list.Item
	for _, list := range fm.Lists {
		allSnippets = append(allSnippets, list.Items()...)
	}
	b, err := json.Marshal(allSnippets)
	if err != nil {
		return err
	}
	err = os.WriteFile(filepath.Join(config.Home, config.File), b, os.ModePerm)
	if err != nil {
		return err
	}
	return nil
}

func newList(items []list.Item, height int, styles SnippetsBaseStyle) *list.Model {
	items = append([]list.Item(nil), items...)
	sortSnippetItems(items)
	snippetList := list.New(items, snippetDelegate{styles: styles, state: navigatingState}, 25, height)
	snippetList.SetShowHelp(false)
	snippetList.SetShowFilter(false)
	snippetList.SetShowTitle(false)
	snippetList.Styles.StatusBar = lipgloss.NewStyle().Margin(1, 2).Foreground(lipgloss.Color("240")).MaxWidth(35 - 2)
	snippetList.Styles.NoItems = lipgloss.NewStyle().Margin(0, 2).Foreground(lipgloss.Color("8")).MaxWidth(35 - 2)
	snippetList.FilterInput.Prompt = "Find: "
	snippetList.FilterInput.PromptStyle = styles.Title
	snippetList.SetStatusBarItemName("snippet", "snippets")
	snippetList.DisableQuitKeybindings()
	snippetList.Styles.Title = styles.Title
	snippetList.Styles.TitleBar = styles.TitleBar

	return &snippetList
}

func newTextInput(placeholder string) textinput.Model {
	i := textinput.New()
	i.Prompt = ""
	i.PromptStyle = lipgloss.NewStyle().Margin(0, 1)
	i.Placeholder = placeholder
	return i
}
