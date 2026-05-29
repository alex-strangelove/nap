package nap

import (
	"cmp"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/list"
)

const (
	defaultIndexedSnippetStem = "new-snippet"
	defaultIndexedFolderStem  = "new-folder"
	defaultIndexSnippetName   = "01-index"
	defaultIndexLanguage      = "md"
)

type indexedName struct {
	index   int
	width   int
	suffix  string
	indexed bool
}

func parseIndexedName(value string) indexedName {
	parts := strings.SplitN(strings.TrimSpace(value), "-", 2)
	if len(parts) != 2 {
		return indexedName{suffix: strings.ToLower(strings.TrimSpace(value))}
	}

	index, err := strconv.Atoi(parts[0])
	if err != nil {
		return indexedName{suffix: strings.ToLower(strings.TrimSpace(value))}
	}

	return indexedName{
		index:   index,
		width:   len(parts[0]),
		suffix:  strings.ToLower(parts[1]),
		indexed: true,
	}
}

func compareIndexedLabels(left, right string) int {
	l := parseIndexedName(left)
	r := parseIndexedName(right)

	switch {
	case l.indexed && r.indexed:
		if diff := cmp.Compare(l.index, r.index); diff != 0 {
			return diff
		}
	case l.indexed:
		return -1
	case r.indexed:
		return 1
	}

	if diff := cmp.Compare(l.suffix, r.suffix); diff != 0 {
		return diff
	}

	return cmp.Compare(strings.ToLower(left), strings.ToLower(right))
}

func compareSnippets(left, right Snippet) int {
	if diff := compareIndexedLabels(left.Name, right.Name); diff != 0 {
		return diff
	}
	if diff := cmp.Compare(strings.ToLower(left.Language), strings.ToLower(right.Language)); diff != 0 {
		return diff
	}
	return cmp.Compare(strings.ToLower(left.File), strings.ToLower(right.File))
}

func sortSnippets(snippets []Snippet) {
	slices.SortFunc(snippets, compareSnippets)
}

func sortSnippetItems(items []list.Item) {
	slices.SortFunc(items, func(left, right list.Item) int {
		ls, lok := left.(Snippet)
		rs, rok := right.(Snippet)
		switch {
		case lok && rok:
			return compareSnippets(ls, rs)
		case lok:
			return -1
		case rok:
			return 1
		default:
			return 0
		}
	})
}

func sortSnippetList(li *list.Model) {
	if li == nil {
		return
	}

	items := append([]list.Item(nil), li.Items()...)
	if len(items) < 2 {
		return
	}

	selectedPath := ""
	if snippet, ok := li.SelectedItem().(Snippet); ok {
		selectedPath = snippet.Path()
	}

	sortSnippetItems(items)
	_ = li.SetItems(items)

	if selectedPath == "" {
		return
	}

	for idx, item := range items {
		snippet, ok := item.(Snippet)
		if ok && snippet.Path() == selectedPath {
			li.Select(idx)
			return
		}
	}
}

func nextIndexedLabel(existing []string, stem string) string {
	maxIndex := 0
	width := 2

	for _, value := range existing {
		parsed := parseIndexedName(value)
		if !parsed.indexed {
			continue
		}
		if parsed.index > maxIndex {
			maxIndex = parsed.index
		}
		if parsed.width > width {
			width = parsed.width
		}
	}

	next := maxIndex + 1
	if nextWidth := len(strconv.Itoa(next)); nextWidth > width {
		width = nextWidth
	}
	return fmt.Sprintf("%0*d-%s", width, next, stem)
}

func nextIndexedSnippetName(items []list.Item, stem string) string {
	existing := make([]string, 0, len(items))
	for _, item := range items {
		snippet, ok := item.(Snippet)
		if ok {
			existing = append(existing, snippet.Name)
		}
	}

	return nextIndexedLabel(existing, stem)
}

func nextIndexedFolderName(children []Folder, stem string) string {
	return nextIndexedMixedName(nil, children, stem)
}

func nextIndexedMixedName(items []list.Item, children []Folder, stem string) string {
	existing := make([]string, 0, len(children))
	for _, item := range items {
		snippet, ok := item.(Snippet)
		if ok {
			existing = append(existing, snippet.Name)
		}
	}
	for _, child := range children {
		existing = append(existing, filepath.Base(string(child)))
	}

	return nextIndexedLabel(existing, stem)
}

func defaultFolderIndexContent(folder Folder) string {
	label := filepath.Base(string(folder))
	parsed := parseIndexedName(label)
	if parsed.indexed && parsed.suffix != "" {
		label = parsed.suffix
	}

	title := strings.TrimSpace(strings.ReplaceAll(label, "-", " "))
	if title == "" {
		title = "New Folder"
	}

	return "# " + strings.Title(title) + "\n"
}
